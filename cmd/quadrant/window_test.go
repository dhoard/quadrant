package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-macos/accessibility"
)

// fakeWindow is a window handle that answers without a window server, which is
// the only way to test the fallback: it exists for applications whose live
// state cannot be summoned on demand.
type fakeWindow struct {
	role    string
	subrole string
	roleErr error

	info    accessibility.WindowInfo
	infoErr error

	closed bool
}

func (f *fakeWindow) Role() (role, subrole string, err error) { return f.role, f.subrole, f.roleErr }

func (f *fakeWindow) Info() (accessibility.WindowInfo, error) { return f.info, f.infoErr }

func (f *fakeWindow) Title() string { return f.info.Title }

func (f *fakeWindow) Close() error {
	f.closed = true
	return nil
}

func (f *fakeWindow) Frame() (accessibility.Rect, error)    { return f.info.Frame, nil }
func (f *fakeWindow) SetPosition(accessibility.Point) error { return nil }
func (f *fakeWindow) SetSize(accessibility.Size) error      { return nil }
func (f *fakeWindow) Raise() error                          { return nil }

// applicationWindow is a standard, unminimized window with the given title.
func applicationWindow(title string) *fakeWindow {
	return &fakeWindow{role: accessibility.RoleWindow, subrole: "AXStandardWindow", info: accessibility.WindowInfo{Title: title}}
}

// desktop is what the Finder answers kAXWindowsAttribute with: an element that
// covers every display and is not a window at all.
func desktop() *fakeWindow {
	return &fakeWindow{role: "AXScrollArea"}
}

// stubAccess replaces the Accessibility queries for one test. The fallback is
// only reachable through them, so a test describes the machine the program must
// cope with rather than depending on the one it runs on.
func stubAccess(t *testing.T, stub lookup) {
	t.Helper()
	previous := access
	access = stub
	t.Cleanup(func() { access = previous })
}

// unanswerable is the failure the fallback exists for: macOS replies to the
// system-wide query with no value at all.
func unanswerable() error {
	return accessibility.AXError(accessibility.AXNoValue).
		Err("reading kAXFocusedApplicationAttribute")
}

// frontmostChrome describes the application the fallback tests below must cope
// with: a Chromium application that does not answer the focused-application
// query, owning a desktop-like element that is not a window, a minimized
// window, and the window in use.
func frontmostChrome() (app accessibility.Application, minimized, inUse *fakeWindow) {
	app = accessibility.Application{PID: 4242, Name: "Chrome", Bundle: "com.google.Chrome", Active: true}
	minimized = applicationWindow("Downloads")
	minimized.info.Minimized = true
	inUse = applicationWindow("Inbox")
	return app, minimized, inUse
}

// The direct query is the first choice, always, and an application that answers
// it must not be sent down the fallback: that is what keeps iTerm2 and every
// other native application on exactly the path it took before this existed.
func TestFocusedWindowPrefersTheDirectQuery(t *testing.T) {
	direct := applicationWindow("Terminal")
	stubAccess(t, lookup{
		focused: func() (window, error) { return direct, nil },
		applications: func() ([]accessibility.Application, error) {
			t.Fatal("the fallback listed applications")
			return nil, nil
		},
		windowsOf: func(int, string) ([]window, error) {
			t.Fatal("the fallback listed windows")
			return nil, nil
		},
	})

	got, err := focusedWindow()
	if err != nil {
		t.Fatalf("focusedWindow: %v", err)
	}
	if got != window(direct) {
		t.Errorf("focusedWindow returned %v, want the directly queried window", got)
	}
	if direct.closed {
		t.Error("focusedWindow closed the window it returned")
	}
}

// The failure that prompted the fallback: the frontmost application's own
// window survives, and every other handle taken on the way is released.
func TestFocusedWindowFallsBackToTheFrontmostApplication(t *testing.T) {
	chrome, minimized, inUse := frontmostChrome()
	directErr := unanswerable()
	notAWindow := desktop()
	var asked struct {
		pid  int
		name string
	}
	stubAccess(t, lookup{
		focused: func() (window, error) { return nil, directErr },
		applications: func() ([]accessibility.Application, error) {
			return []accessibility.Application{
				{PID: 1, Name: "Window Server"},
				chrome,
			}, nil
		},
		windowsOf: func(pid int, name string) ([]window, error) {
			asked.pid, asked.name = pid, name
			return []window{notAWindow, minimized, inUse}, nil
		},
	})

	got, err := focusedWindow()
	if err != nil {
		t.Fatalf("focusedWindow: %v", err)
	}
	if got != window(inUse) {
		t.Errorf("focusedWindow returned %v, want the unminimized window", got)
	}
	if asked.pid != chrome.PID || asked.name != chrome.Name {
		t.Errorf("listed windows of pid %d %q, want pid %d %q", asked.pid, asked.name, chrome.PID, chrome.Name)
	}
	for _, w := range []*fakeWindow{notAWindow, minimized} {
		if !w.closed {
			t.Errorf("handle for %q was not closed", w.Title())
		}
	}
	if inUse.closed {
		t.Error("the chosen window was closed; the caller moves it and closes it itself")
	}
}

// A window the caller cannot measure cannot be moved, so it is passed over
// rather than chosen and failed at.
func TestFallbackSkipsWindowsThatCannotBeRead(t *testing.T) {
	chrome, _, inUse := frontmostChrome()
	unreadable := &fakeWindow{role: accessibility.RoleWindow, infoErr: errors.New("reading kAXPositionAttribute: AXError -25205: the element does not have that attribute")}
	unroled := &fakeWindow{roleErr: accessibility.AXError(accessibility.AXCannotComplete).Err("reading kAXRoleAttribute")}
	stubAccess(t, lookup{
		focused:      func() (window, error) { return nil, unanswerable() },
		applications: func() ([]accessibility.Application, error) { return []accessibility.Application{chrome}, nil },
		windowsOf:    func(int, string) ([]window, error) { return []window{unroled, unreadable, inUse}, nil },
	})

	got, err := focusedWindow()
	if err != nil {
		t.Fatalf("focusedWindow: %v", err)
	}
	if got != window(inUse) {
		t.Errorf("focusedWindow returned %v, want %q", got, inUse.Title())
	}
	for _, w := range []*fakeWindow{unreadable, unroled} {
		if !w.closed {
			t.Error("an unusable handle was not closed")
		}
	}
}

// An application whose only windows are minimized still gets one moved: a
// minimized window keeps a position and appears where it was put when restored,
// which is a better answer than an error.
func TestFallbackUsesAMinimizedWindowWhenItIsAllThereIs(t *testing.T) {
	chrome, minimized, _ := frontmostChrome()
	second := applicationWindow("Calendar")
	second.info.Minimized = true
	stubAccess(t, lookup{
		focused:      func() (window, error) { return nil, unanswerable() },
		applications: func() ([]accessibility.Application, error) { return []accessibility.Application{chrome}, nil },
		windowsOf:    func(int, string) ([]window, error) { return []window{minimized, second}, nil },
	})

	got, err := focusedWindow()
	if err != nil {
		t.Fatalf("focusedWindow: %v", err)
	}
	if got != window(minimized) {
		t.Errorf("focusedWindow returned %v, want the first window in the list", got)
	}
	if !second.closed {
		t.Error("the window that was not chosen was not closed")
	}
}

// A nil handle in the list is not a window and must not be called. A nil that
// reached the interface would make every method call a nil-pointer panic.
func TestPickWindowIgnoresNilHandles(t *testing.T) {
	inUse := applicationWindow("Inbox")
	got, err := pickWindow([]window{nil, inUse})
	if err != nil {
		t.Fatalf("pickWindow: %v", err)
	}
	if got != window(inUse) {
		t.Errorf("pickWindow returned %v, want %q", got, inUse.Title())
	}
}

// Nothing movable is a refusal that says so, and it does not leak the handles it
// looked at.
func TestPickWindowWithNothingMovable(t *testing.T) {
	windows := []window{desktop(), nil}
	if _, err := pickWindow(windows); !errors.Is(err, accessibility.ErrNoWindow) {
		t.Fatalf("pickWindow error = %v, want %v", err, accessibility.ErrNoWindow)
	}
	if !windows[0].(*fakeWindow).closed {
		t.Error("the handle that was not chosen was not closed")
	}
}

// When both routes fail, the error carries both reasons: the first names the
// query macOS did not answer, and the second names why the application's own
// windows could not stand in for it.
func TestFocusedWindowReportsBothRoutes(t *testing.T) {
	directErr := unanswerable()
	stubAccess(t, lookup{
		focused: func() (window, error) { return nil, directErr },
		applications: func() ([]accessibility.Application, error) {
			return []accessibility.Application{{PID: 4242, Name: "Chrome", Active: true}}, nil
		},
		windowsOf: func(int, string) ([]window, error) { return nil, accessibility.ErrNoWindow },
	})

	_, err := focusedWindow()
	if err == nil {
		t.Fatal("focusedWindow succeeded with neither route available")
	}
	for _, want := range []string{directErr.Error(), "frontmost application fallback", accessibility.ErrNoWindow.Error()} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

// The handle a failed lookup did take is released before the error comes back,
// so a shortcut that cannot move anything does not accumulate AX references.
func TestFocusedWindowClosesWindowsWhenItGivesUp(t *testing.T) {
	notAWindow := desktop()
	stubAccess(t, lookup{
		focused: func() (window, error) { return nil, unanswerable() },
		applications: func() ([]accessibility.Application, error) {
			return []accessibility.Application{{PID: 4242, Name: "Finder", Active: true}}, nil
		},
		windowsOf: func(int, string) ([]window, error) { return []window{notAWindow}, nil },
	})

	if _, err := focusedWindow(); err == nil {
		t.Fatal("focusedWindow succeeded with nothing movable")
	}
	if !notAWindow.closed {
		t.Error("the rejected handle was not closed")
	}
}

// A frontmost application is required before any window is listed; without one,
// the error says so rather than moving some other application's window.
func TestFallbackWithoutAFrontmostApplication(t *testing.T) {
	stubAccess(t, lookup{
		focused: func() (window, error) { return nil, unanswerable() },
		applications: func() ([]accessibility.Application, error) {
			return []accessibility.Application{{PID: 1, Name: "Window Server"}}, nil
		},
		windowsOf: func(int, string) ([]window, error) {
			t.Fatal("listed windows without a frontmost application")
			return nil, nil
		},
	})

	_, err := focusedWindow()
	if err == nil {
		t.Fatal("focusedWindow succeeded with no frontmost application")
	}
	if !strings.Contains(err.Error(), "frontmost") {
		t.Errorf("error %q does not say no application was frontmost", err)
	}
}

// An empty handle is a failed lookup even when no error comes with it, so that a
// caller never dereferences nothing.
func TestFocusedWindowTreatsAnEmptyHandleAsFailure(t *testing.T) {
	inUse := applicationWindow("Inbox")
	stubAccess(t, lookup{
		focused: func() (window, error) { return nil, nil },
		applications: func() ([]accessibility.Application, error) {
			return []accessibility.Application{{PID: 7, Name: "Chrome", Active: true}}, nil
		},
		windowsOf: func(int, string) ([]window, error) { return []window{inUse}, nil },
	})

	got, err := focusedWindow()
	if err != nil {
		t.Fatalf("focusedWindow: %v", err)
	}
	if got != window(inUse) {
		t.Errorf("focusedWindow returned %v, want %q", got, inUse.Title())
	}
}

// A handle that arrives together with an error is released rather than left to
// the window server, and the fallback runs as it would for the error alone.
func TestFocusedWindowClosesAnUnusableDirectHandle(t *testing.T) {
	stale := applicationWindow("Closed")
	inUse := applicationWindow("Inbox")
	stubAccess(t, lookup{
		focused: func() (window, error) {
			return stale, accessibility.AXError(accessibility.AXInvalidUIElement).Err("reading kAXFocusedWindowAttribute")
		},
		applications: func() ([]accessibility.Application, error) {
			return []accessibility.Application{{PID: 4242, Name: "Chrome", Active: true}}, nil
		},
		windowsOf: func(int, string) ([]window, error) { return []window{inUse}, nil },
	})

	got, err := focusedWindow()
	if err != nil {
		t.Fatalf("focusedWindow: %v", err)
	}
	if got != window(inUse) {
		t.Errorf("focusedWindow returned %v, want %q", got, inUse.Title())
	}
	if !stale.closed {
		t.Error("the handle that came back with the error was not closed")
	}
}

// Each failure of the second route names what failed, so a log reader can tell an
// application that will not answer from an application that has no window.
func TestFallbackReportsWhereItFailed(t *testing.T) {
	apps := func() ([]accessibility.Application, error) {
		return []accessibility.Application{{PID: 4242, Name: "Chrome", Active: true}}, nil
	}
	tests := []struct {
		name    string
		stub    lookup
		wantErr []string
	}{
		{
			name: "the application list cannot be read",
			stub: lookup{
				focused:      func() (window, error) { return nil, unanswerable() },
				applications: func() ([]accessibility.Application, error) { return nil, accessibility.ErrUnsupported },
				windowsOf: func(int, string) ([]window, error) {
					t.Fatal("listed windows after the application list failed")
					return nil, nil
				},
			},
			wantErr: []string{"listing applications", accessibility.ErrUnsupported.Error()},
		},
		{
			name: "the application will not answer for its windows",
			stub: lookup{
				focused:      func() (window, error) { return nil, unanswerable() },
				applications: apps,
				windowsOf: func(int, string) ([]window, error) {
					return nil, accessibility.AXError(accessibility.AXCannotComplete).Err("reading kAXWindowsAttribute of pid 4242")
				},
			},
			wantErr: []string{"Chrome (pid 4242)", "kAXWindowsAttribute", "frontmost application fallback"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stubAccess(t, tc.stub)
			_, err := focusedWindow()
			if err == nil {
				t.Fatal("focusedWindow succeeded with neither route available")
			}
			for _, want := range tc.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestFrontmostApp(t *testing.T) {
	apps := []accessibility.Application{
		{PID: 1, Name: "Window Server"},
		{PID: 2, Name: "Finder", Active: true},
		{PID: 3, Name: "Chrome", Active: true},
	}
	got, ok := frontmostApp(apps)
	if !ok || got.PID != 2 {
		t.Errorf("frontmostApp = %v, %v; want the first application reporting itself active", got, ok)
	}
	if _, ok := frontmostApp(nil); ok {
		t.Error("frontmostApp found an application in an empty list")
	}
}
