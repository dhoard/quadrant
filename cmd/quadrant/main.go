// quadrant provides global shortcuts for arranging the focused macOS window
// into quadrants or screen halves. It is a background accessory app, not a full
// window manager. While it runs it re-reads its configuration file every 30
// seconds, so shortcut and padding changes apply without a restart. An icon in
// the menu bar carries About and Quit; nothing else is on screen.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-macos/accessibility"
	"github.com/go-macos/hotkey"
	"github.com/go-macos/objc"

	"quadrant/internal/config"
	"quadrant/internal/geometry"
	"quadrant/internal/grant"
	"quadrant/internal/macos"
	"quadrant/internal/shortcut"
)

const (
	// name is what the About panel and the menu bar icon call the program.
	name = "Quadrant"

	// credits is the small print under the version in the About panel.
	credits = "© 2026 quadrant contributors · MIT licensed"

	// grantPoll is how often a running program that does not hold the
	// Accessibility grant asks macOS whether it has appeared. It is the silent
	// check, which shows no dialog, so the wait costs the user nothing.
	grantPoll = 2 * time.Second
)

// version is the version the About panel shows. build.sh replaces it with the
// build's git description; a build made without git says so.
var version = "dev"

type binding struct {
	placement geometry.Placement
	combo     hotkey.Combo
	label     string
}

// service holds the configuration that is in service and the hot keys that are
// currently registered for it. The reload watcher replaces both while the run
// loop is going, so every access is either guarded by mu or atomic.
type service struct {
	opts *hotkey.Options

	// live is the configuration in service. Presses read the padding from it,
	// so a reload takes effect on the next press rather than at the next start.
	live atomic.Pointer[config.Configuration]

	mu sync.Mutex

	// granted is whether the Accessibility grant exists. Hot keys are not
	// claimed without it: a shortcut that cannot move a window answers every
	// press with an error instead of working, so the keys wait, and grant
	// claims them the moment permission arrives.
	granted  bool
	bindings []binding
	keys     []*hotkey.Hotkey
}

func (s *service) padding() float64 {
	if cfg := s.live.Load(); cfg != nil {
		return cfg.PaddingPoints
	}
	return 0
}

func (s *service) dispatch(h *hotkey.Hotkey, placement geometry.Placement) {
	for range h.C() {
		if err := place(placement, s.padding()); err != nil {
			log.Printf("%s: %v", placement, err)
		}
	}
}

// registerAll claims every binding and starts its placement goroutine. It keeps
// the keys claimed so far in p, so a caller that sees an error can release them
// rather than leak them. Callers hold s.mu.
func (s *service) registerAll(bindings []binding) error {
	for _, b := range bindings {
		h, err := hotkey.Register(b.combo, s.opts)
		if err != nil {
			return fmt.Errorf("register %s (%s): %w", b.placement, b.label, err)
		}
		s.keys = append(s.keys, h)
		s.bindings = append(s.bindings, b)
		log.Printf("registered %s → %s", h.Combo().Glyphs(), b.placement)
		go s.dispatch(h, b.placement)
	}
	return nil
}

// closeAll releases every registered hot key. Closing a hot key closes the
// channel its placement goroutine ranges over, so those goroutines end here.
// Hotkey.Close is safe to call twice, which keeps the error paths simple.
// Callers hold s.mu.
func (s *service) closeAll() {
	for _, h := range s.keys {
		_ = h.Close()
	}
	s.keys = nil
	s.bindings = nil
}

// swap replaces the registered hot keys with a new set. The previous set is
// copied first, because its combinations have to be claimed again if the new
// set cannot be. A partial registration is released before returning.
// Callers hold s.mu.
func (s *service) swap(bindings []binding) error {
	previous := append([]binding(nil), s.bindings...)
	s.closeAll()
	if err := s.registerAll(bindings); err != nil {
		s.closeAll()
		// The previous combinations were released a moment ago, so they are
		// free again; the only way this fails is another process claiming one
		// in that instant. Either way the user is told, and is not left with
		// fewer shortcuts than they had.
		if restoreErr := s.registerAll(previous); restoreErr != nil {
			return fmt.Errorf("%w; the previous shortcuts could not be restored: %v", err, restoreErr)
		}
		return err
	}
	return nil
}

// apply puts cfg into service. The configuration has already been loaded and
// validated by the watcher; parseBindings re-checks the parts only the hot key
// layer can judge (unknown names, combinations used twice) and runs BEFORE any
// live hot key is touched, so a configuration that cannot be applied leaves the
// running one exactly as it was.
func (s *service) apply(cfg config.Configuration) error {
	bindings, err := parseBindings(cfg)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// With no grant there is nothing to register; the configuration is kept and
	// grant applies it when permission arrives.
	if s.granted {
		if err := s.swap(bindings); err != nil {
			return err
		}
	}
	s.live.Store(&cfg)
	return nil
}

// grant records that the Accessibility grant has been given and claims the
// shortcuts for the configuration in service. It is the one call that turns a
// running program without permission into a working one, without a restart.
func (s *service) grant() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.granted {
		return nil
	}
	s.granted = true
	cfg := s.live.Load()
	if cfg == nil {
		return nil
	}
	bindings, err := parseBindings(*cfg)
	if err != nil {
		return err
	}
	return s.swap(bindings)
}

// close releases the live hot keys at shutdown.
func (s *service) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeAll()
}

func parseBindings(cfg config.Configuration) ([]binding, error) {
	result := make([]binding, 0, 8)
	seen := map[hotkey.Combo]string{}
	for _, entry := range cfg.Shortcuts.Entries() {
		// Half-screen placements are optional: an empty binding disables it.
		if strings.TrimSpace(entry.Binding) == "" {
			continue
		}
		combo, err := shortcut.Parse(entry.Binding)
		if err != nil {
			return nil, fmt.Errorf("shortcuts.%s: %w", entry.Name, err)
		}
		if other, found := seen[combo]; found {
			return nil, fmt.Errorf("shortcuts.%s duplicates shortcuts.%s", entry.Name, other)
		}
		seen[combo] = entry.Name
		result = append(result, binding{placement: geometry.Placement(entry.Name), combo: combo, label: entry.Binding})
	}
	return result, nil
}

func place(placement geometry.Placement, padding float64) error {
	w, err := accessibility.FocusedWindow()
	if err != nil {
		return fmt.Errorf("focused window: %w", err)
	}
	defer w.Close()
	frame, err := w.Frame()
	if err != nil {
		return fmt.Errorf("read window frame: %w", err)
	}
	displays, err := accessibility.Displays()
	if err != nil {
		return fmt.Errorf("list displays: %w", err)
	}
	display, ok := accessibility.DisplayFor(frame, displays)
	if !ok {
		return fmt.Errorf("no display for focused window")
	}
	area, err := macos.VisibleArea(display)
	if err != nil {
		return fmt.Errorf("usable display bounds: %w", err)
	}
	target, err := geometry.Target(
		geometry.Rect{X: area.X, Y: area.Y, W: area.W, H: area.H},
		placement, padding,
	)
	if err != nil {
		return err
	}
	want := accessibility.Rect{X: target.X, Y: target.Y, W: target.W, H: target.H}
	result, err := accessibility.Move(w, want, &accessibility.Options{NoRaise: true})
	if err != nil {
		return fmt.Errorf("move/resize: %w (%s)", err, result)
	}
	if !result.Resized {
		log.Printf("notice: window could not be resized exactly (application minimum size?): %s", result)
	}
	return nil
}

func run() error {
	runtime.LockOSThread() // Required for the NSApplication / Carbon hotkey run loop.
	defer runtime.UnlockOSThread()

	defaultPath, err := config.DefaultPath()
	if err != nil {
		return err
	}
	configPath := flag.String("config", defaultPath, "configuration JSON path (watched for changes while running)")
	check := flag.Bool("check-config", false, "validate the config and print shortcuts without starting or watching")
	flag.Parse()
	cfg, content, err := config.LoadOrCreate(*configPath)
	if err != nil {
		return err
	}
	bindings, err := parseBindings(cfg)
	if err != nil {
		return err
	}
	log.Printf("configuration: %s", *configPath)
	for _, b := range bindings {
		log.Printf("  %-14s %s", b.placement, b.label)
	}
	if *check {
		log.Print("configuration is valid")
		return nil
	}

	// The Accessibility grant is what lets quadrant move other applications'
	// windows, and macOS gives it to the application that asks for it, not to
	// the process that happens to be running. A program that does not hold it
	// asks once -- the prompting check, the only call here that shows a dialog
	// -- and then keeps going: the menu-bar icon needs no grant, so the menu
	// stays reachable, and the shortcuts are claimed when the grant appears
	// rather than at the next start. Exiting instead, as this program once did,
	// only left launchd restarting something that could never get further, with
	// no icon to quit it from and no way to read the advice quietly.
	trust, err := accessibility.Status()
	if err != nil {
		return fmt.Errorf("Accessibility status: %w", err)
	}
	if !trust.Trusted {
		accessibility.RequestTrust()
		// Advice is the library's wording on purpose: it is the one text that is
		// true for this process, which is a bundle that can be granted under its
		// own name rather than a binary that can only inherit a grant. The status
		// line names the bundle identifier, which is what tells the entry in
		// System Settings apart when two builds of the same program are listed.
		log.Printf("Accessibility permission required: %s", trust.Advice())
		log.Printf("Accessibility status: %s", trust)
	}

	// Refuse to substitute a different key if the desired shortcut is taken.
	// A silent fallback would violate the user's explicit configuration.
	svc := &service{
		opts:    &hotkey.Options{Ladder: []hotkey.Modifier{}, OnThisKeyboard: true},
		granted: trust.Trusted,
	}
	if err := svc.apply(cfg); err != nil {
		return fmt.Errorf("%w (change it in %s)", err, *configPath)
	}
	defer svc.close()

	// Re-read the configuration, and watch for the Accessibility grant, until
	// the program stops. Watch is platform-independent and silent until the file
	// actually changes; hotkey.Register is documented as callable from any
	// goroutine, so the watcher applies reloads itself.
	running, stopRunning := context.WithCancel(context.Background())
	defer stopRunning() // LIFO: stops the watchers before the hot keys are closed
	go config.Watch(running, *configPath, config.ReloadInterval, content, svc.apply)
	if !trust.Trusted {
		go grant.Watch(running, grantPoll, accessibility.Trusted, func() {
			log.Print("Accessibility permission granted; registering the shortcuts")
			if err := svc.grant(); err != nil {
				log.Printf("shortcuts after the Accessibility grant: %v (change it in %s)", err, *configPath)
			}
		})
	}

	// Stop through RunAppLoop's stop predicate rather than -[NSApplication
	// stop:]. RunApp enters -[NSApplication run], which reads the stop flag
	// only after it has dequeued a real NSEvent, so stopping it from a signal
	// hung at 0% CPU until a mouse move arrived. RunAppLoop asks this
	// predicate between bounded 50ms event waits, so Ctrl+C is honoured even
	// with nobody at the machine. The menu's Quit item sets the same flag.
	var quitting atomic.Bool
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	go func() {
		<-stop
		quitting.Store(true)
	}()

	// The icon is the app's only user interface, and the only way to quit it
	// without a Terminal. Best effort all the same: a session with no menu bar
	// still gets its shortcuts, which are the point of the program.
	if removeIcon, err := addMenuBar(&quitting); err != nil {
		log.Printf("no menu bar icon: %v", err)
	} else {
		defer removeIcon()
	}

	log.Print("quadrant running in the menu bar (no Dock icon); Quit from its menu, or Ctrl+C if running in Terminal")
	objc.RunAppLoop(1, func() bool { return quitting.Load() }) // NSApplicationActivationPolicyAccessory
	return nil
}

// addMenuBar puts the menu-bar icon up with About and Quit behind it, and
// returns a function that takes it down again.
func addMenuBar(quitting *atomic.Bool) (func(), error) {
	icon, err := macos.QuadrantIcon()
	if err != nil {
		return nil, err
	}
	item, err := macos.AddStatusItem(icon, name, "QuadrantStatusItem", []macos.MenuAction{
		{Title: "About Quadrant", Do: about},
		{Separator: true},
		// The Accessibility grant is switched on in System Settings, and the
		// user may want that pane again later -- to look at it, or to give the
		// grant a rebuild dropped. This is the same pane the permission dialog
		// opens, reachable without reading the log.
		{Title: "Accessibility Settings…", Do: accessibilitySettings},
		{Separator: true},
		// Quit goes through the flag Ctrl+C sets rather than -[NSApplication
		// terminate:], which would end the process from inside the click and
		// skip every deferred call: the shortcuts would not be released and the
		// watcher would not be stopped. This way out unwinds run() exactly as
		// an interrupt does.
		{Title: "Quit Quadrant", Key: "q", Do: func() {
			log.Print("quitting")
			quitting.Store(true)
		}},
	})
	if err != nil {
		return nil, err
	}
	return item.Remove, nil
}

// accessibilitySettings opens the System Settings pane where the Accessibility
// grant is given. Failing to open it is not fatal: the log and the README name
// the pane in words.
func accessibilitySettings() {
	if err := macos.OpenAccessibilitySettings(); err != nil {
		log.Printf("open Accessibility settings: %v", err)
	}
}

// about shows the standard About panel.
func about() {
	icon, err := macos.AboutIcon()
	if err != nil {
		log.Printf("about: %v", err)
		icon = 0
	}
	if err := macos.AboutPanel(name, version, credits, icon); err != nil {
		log.Printf("about: %v", err)
	}
}

func main() {
	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.SetPrefix("quadrant: ")
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "quadrant:", err)
		os.Exit(1)
	}
}
