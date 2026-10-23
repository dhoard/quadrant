package main

// Finding the window a placement should move.
//
// The direct question is the right one: ask the system-wide Accessibility
// element which application is focused, then ask that application which of its
// windows is. accessibility.FocusedWindow asks exactly that, and for a native
// Cocoa application it is answered every time.
//
// It is not answered by every application. Some return AXError -25212
// (kAXErrorNoValue) for kAXFocusedApplicationAttribute -- macOS replies, and has
// nothing to put in the reply -- so the query fails before any window is named,
// and the failure is about the LOOKUP rather than about the window. Chromium
// applications are where this was first seen; nothing below is specific to
// them, and any application that answers the question this way takes the same
// second route.
//
// That route is the frontmost application's own window list, which needs no
// system-wide query at all: the windows an application owns can always be
// listed by its pid. What it cannot say is which of them has focus, so the
// choice is made from what the elements themselves report. See pickWindow.

import (
	"errors"
	"fmt"
	"log"

	"github.com/go-macos/accessibility"
)

// window is what placing a window needs from a window handle: measure it, move
// it, and ask what it is.
//
// The implementation outside these two files is *accessibility.AXWindow, which
// is the only thing that can talk to the window server. It is an interface so
// that the fallback below can be tested at all: the applications it exists for
// are not always running on the machine the tests are run on, and waiting for
// one to misbehave is not a test.
type window interface {
	accessibility.Window // Frame, SetPosition, SetSize, Raise: what placing needs

	// Role reports kAXRole and kAXSubrole: "AXWindow" and "AXStandardWindow"
	// for an ordinary window.
	Role() (role, subrole string, err error)

	// Info is a fresh snapshot of the window: whether it is minimized, and
	// enough to name it in a log line.
	Info() (accessibility.WindowInfo, error)

	// Title is the title as it was when the handle was made, for a log line.
	Title() string

	// Close releases the handle. AXUIElementRefs are not garbage collected,
	// so every handle taken here is closed, chosen or not.
	Close() error
}

// lookup holds the Accessibility queries the window lookup makes. It is a value
// so that a test can answer the three of them itself; access is what the
// program uses, and nothing else in this package calls the library directly.
type lookup struct {
	// focused is the direct route: the system-wide element's idea of the
	// focused window.
	focused func() (window, error)

	// applications lists the running applications. One of them reports
	// which is frontmost.
	applications func() ([]accessibility.Application, error)

	// windowsOf lists one application's own windows by pid.
	windowsOf func(pid int, appName string) ([]window, error)
}

// access is the live lookup: the library's queries, and nothing else.
var access = lookup{
	focused: func() (window, error) {
		w, err := accessibility.FocusedWindow()
		if err != nil {
			return nil, err
		}
		if w == nil { // cannot happen; a nil handle must not look like success
			return nil, accessibility.ErrNoWindow
		}
		return w, nil
	},
	applications: accessibility.Applications,
	windowsOf: func(pid int, appName string) ([]window, error) {
		windows, err := accessibility.WindowsOf(pid, appName)
		if err != nil {
			return nil, err
		}
		out := make([]window, len(windows))
		for i, w := range windows {
			// A nil handle stays nil here rather than becoming a non-nil
			// interface holding a nil pointer, which every method would
			// then be called on.
			if w != nil {
				out[i] = w
			}
		}
		return out, nil
	},
}

// focusedWindow returns a handle on the window a placement should move, and the
// caller closes it.
//
// The direct query is asked first and unconditionally, so an application that
// answers it takes exactly the route it took before the fallback existed. Only
// when that fails -- for any reason, not only kAXErrorNoValue -- is the
// frontmost application's window list used instead.
func focusedWindow() (window, error) {
	w, err := access.focused()
	if err == nil && w != nil {
		return w, nil
	}
	if w != nil {
		// A handle that came back with an error is not usable, but it is
		// still a reference the window server keeps alive: release it.
		_ = w.Close()
	}
	if err == nil {
		err = accessibility.ErrNoWindow
	}
	w, fallbackErr := frontmostWindow()
	if fallbackErr != nil {
		// Both are reported. The first says what went wrong with the query
		// macOS is supposed to answer, which is the bug worth fixing; the
		// second says why the application's own window list could not serve
		// as the answer either. Reporting only the second would hide the
		// first, and reporting only the first explains nothing about what
		// the program then tried.
		return nil, fmt.Errorf("%w; frontmost application fallback: %w", err, fallbackErr)
	}
	return w, nil
}

// frontmostWindow returns the window to move from the frontmost application's
// own window list. It is the second route; nothing here asks macOS which
// application is focused.
func frontmostWindow() (window, error) {
	apps, err := access.applications()
	if err != nil {
		return nil, fmt.Errorf("listing applications: %w", err)
	}
	app, ok := frontmostApp(apps)
	if !ok {
		return nil, errors.New("no running application reports itself as frontmost")
	}
	windows, err := access.windowsOf(app.PID, app.Name)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", describeApp(app), err)
	}
	w, err := pickWindow(windows)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", describeApp(app), err)
	}
	// The window that has focus is exactly what the direct query would have
	// named, so this is a choice made from the list rather than an answer: the
	// log says which one it was. A window moving when another was meant to is
	// otherwise indistinguishable from the program working.
	log.Printf("notice: focused window unavailable; using %q of %s, the frontmost application",
		w.Title(), describeApp(app))
	return w, nil
}

// frontmostApp returns the application the system reports as frontmost.
//
// Two can claim it in the instant an application switch is under way; the first
// is as good a guess as the second, and better than moving nothing.
func frontmostApp(apps []accessibility.Application) (accessibility.Application, bool) {
	for _, app := range apps {
		if app.Active {
			return app, true
		}
	}
	return accessibility.Application{}, false
}

// pickWindow chooses the window to move from one application's window list, and
// closes every handle it does not choose.
//
// A list of an application's windows is not a list of windows: the Finder
// answers kAXWindowsAttribute with its desktop, an AXScrollArea covering every
// display, and moving that fails after two attempts and blames a window that
// never existed. The role decides, whatever list an element arrived in.
//
// Among actual windows, an unminimized one comes first, because it is what the
// person pressing the shortcut can see. A minimized window is taken when it is
// all the application has: it keeps a position, and appears where it was put
// when it is restored -- which is a better answer than refusing to move
// anything.
func pickWindow(windows []window) (window, error) {
	chosen := -1
	for i, w := range windows {
		if w == nil {
			continue
		}
		if role, _, err := w.Role(); err != nil || role != accessibility.RoleWindow {
			continue
		}
		info, err := w.Info()
		if err != nil {
			// An element that cannot report kAXPositionAttribute and
			// kAXSizeAttribute cannot be measured or moved either.
			continue
		}
		if chosen < 0 {
			chosen = i // the first real window, minimized or not
		}
		if !info.Minimized {
			chosen = i
			break
		}
	}
	closeAllBut(windows, chosen)
	if chosen < 0 {
		return nil, accessibility.ErrNoWindow
	}
	return windows[chosen], nil
}

// closeAllBut releases every handle in the list except windows[keep]. Closing a
// handle twice is allowed, so a handle already released above is not a special
// case.
func closeAllBut(windows []window, keep int) {
	for i, w := range windows {
		if w == nil || i == keep {
			continue
		}
		_ = w.Close()
	}
}

// describeApp names an application for an error message: the name a person
// recognises, and the pid that is unambiguous even when the name is empty, as
// it is for the entries the window server contributes.
func describeApp(app accessibility.Application) string {
	name := app.Name
	if name == "" {
		name = "application"
	}
	return fmt.Sprintf("%s (pid %d)", name, app.PID)
}
