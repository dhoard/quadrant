package macos

import (
	"errors"
	"fmt"

	"github.com/go-macos/objc"
)

// The menu-bar extra: the icon in the status area at the right of the menu bar
// and the menu behind it.
//
// None of this has a C-level equivalent, so the pieces are reached through objc
// like the rest of the package: a status item from the system status bar, a
// menu target registered at runtime whose methods are Go closures, and the
// standard About panel. It lives here rather than in cmd/quadrant so the
// Objective-C stays next to the other platform code, and so the app's startup
// path stays readable.
//
// Everything here must run on the process main thread -- the one
// objc.RunAppLoop owns -- and that is why callers reach it directly instead of
// through objc.DispatchMain: DispatchMain queues its block asynchronously, and
// before the run loop is going, a queued block never runs.

// Icon geometry, in points. A menu-bar extra draws about as much ink as a
// capital letter: an 18pt canvas, the square inset from its edge, and one line
// weight for the outline and the cross alike, because the four quadrants are
// what the lines divide rather than four boxes.
const (
	iconPoints = 18.0
	iconInset  = 1.5
	iconWeight = 1.5

	// aboutPoints is the size of the icon the About panel shows. macOS app
	// icons are 128pt at the smallest and are drawn well inside their canvas.
	aboutPoints = 128.0

	// retina is the pixel density every Mac this program runs on has. The
	// images are drawn at this scale rather than enlarged to it.
	retina = 2
)

// menuTargetClass is the runtime class carrying the menu's actions. It is
// named for the app so that it cannot collide with a class AppKit already has,
// and so a second call to AddStatusItem fails loudly on the name rather than
// silently replacing the first menu's methods.
const menuTargetClass = "QuadrantMenuTarget"

// MenuAction is one entry of the menu behind a menu-bar extra. Key is a key
// equivalent, shown and accepted while the menu is open ("q" for a Quit item);
// Do runs on the main thread, inside the menu's own event tracking. A Separator
// entry draws a dividing line and ignores the other fields.
type MenuAction struct {
	Title     string
	Key       string
	Do        func()
	Separator bool
}

// StatusItem is a menu-bar extra: an icon in the status area with a menu.
// AppKit keeps the item and everything it draws; the status item's Go side is
// only needed to take the icon down again.
type StatusItem struct {
	item objc.ID
}

// AddStatusItem puts icon in the menu bar with actions behind it and returns
// the item, so a caller that stops before the process does can remove it again.
//
// tooltip is the text shown on hover, and the label VoiceOver reads when it
// reaches the extra. autosaveName lets macOS remember where the user dragged
// the icon (Command-drag); an empty name leaves its position to the system.
func AddStatusItem(icon objc.ID, tooltip, autosaveName string, actions []MenuAction) (*StatusItem, error) {
	if icon == 0 {
		return nil, errors.New("no icon to draw")
	}
	if err := objc.Load(objc.AppKit); err != nil {
		return nil, fmt.Errorf("load AppKit: %w", err)
	}
	for _, action := range actions {
		if action.Separator || action.Do != nil {
			continue
		}
		return nil, fmt.Errorf("menu action %q has no function to run", action.Title)
	}

	bar := objc.ClassID("NSStatusBar").Send(objc.Sel("systemStatusBar"))
	if bar == 0 {
		// No menu bar to put an icon in: an SSH session, or a launch agent
		// running with nobody logged in.
		return nil, errors.New("this session has no status bar")
	}
	// NSVariableStatusItemLength (-1): as wide as the icon needs.
	item := bar.Send(objc.Sel("statusItemWithLength:"), float64(-1))
	if item == 0 {
		return nil, errors.New("AppKit returned no status item")
	}
	remove := func() { bar.Send(objc.Sel("removeStatusItem:"), item) }

	button := item.Send(objc.Sel("button"))
	if button == 0 {
		remove()
		return nil, errors.New("status item has no button to draw in")
	}
	button.Send(objc.Sel("setImage:"), icon)
	if tooltip != "" {
		button.Send(objc.Sel("setToolTip:"), objc.NSString(tooltip))
		button.Send(objc.Sel("setAccessibilityLabel:"), objc.NSString(tooltip))
	}
	if autosaveName != "" {
		item.Send(objc.Sel("setAutosaveName:"), objc.NSString(autosaveName))
	}

	target, err := menuTarget(actions)
	if err != nil {
		remove()
		return nil, err
	}
	menu, err := buildMenu(actions, target)
	if err != nil {
		remove()
		return nil, err
	}
	item.Send(objc.Sel("setMenu:"), menu)
	return &StatusItem{item: item}, nil
}

// Remove takes the icon out of the menu bar. The process ending does this too;
// Remove is for a caller that wants it gone while it keeps running.
func (s *StatusItem) Remove() {
	if s == nil || s.item == 0 {
		return
	}
	objc.ClassID("NSStatusBar").Send(objc.Sel("systemStatusBar")).Send(objc.Sel("removeStatusItem:"), s.item)
	s.item = 0
}

// menuTarget registers an Objective-C class whose methods are the menu's
// actions and returns one instance of it to be the items' target.
func menuTarget(actions []MenuAction) (objc.ID, error) {
	methods := make([]objc.MethodDef, 0, len(actions))
	for i, action := range actions {
		if action.Separator {
			continue
		}
		run := action.Do
		methods = append(methods, objc.MethodDef{
			Cmd: actionSelector(i),
			// The receiver and selector are passed in but unused: the closure
			// already knows which action it runs.
			Fn: func(objc.ID, objc.SEL) { run() },
		})
	}
	class, err := objc.RegisterClass(menuTargetClass, objc.GetClass("NSObject"), methods)
	if err != nil {
		return 0, fmt.Errorf("register the menu target: %w", err)
	}
	target := objc.ID(class).Send(objc.Sel("alloc")).Send(objc.Sel("init"))
	if target == 0 {
		return 0, errors.New("the menu target could not be created")
	}
	// NSMenuItem's target is a weak reference, so nothing in AppKit keeps this
	// object alive. Leaving it unretained would be a lifetime bug decided by
	// whichever garbage collection ran first, which is why the retain is here
	// rather than at the call site.
	target.Send(objc.Sel("retain"))
	return target, nil
}

// buildMenu builds the menu behind the icon. Every item is given an explicit
// target, so its action reaches our own method rather than a responder chain
// with no window in it.
func buildMenu(actions []MenuAction, target objc.ID) (objc.ID, error) {
	menu := objc.ClassID("NSMenu").Send(objc.Sel("alloc")).Send(objc.Sel("init"))
	if menu == 0 {
		return 0, errors.New("no menu")
	}
	for i, action := range actions {
		if action.Separator {
			menu.Send(objc.Sel("addItem:"), objc.ClassID("NSMenuItem").Send(objc.Sel("separatorItem")))
			continue
		}
		item := objc.ClassID("NSMenuItem").Send(objc.Sel("alloc")).Send(
			objc.Sel("initWithTitle:action:keyEquivalent:"),
			objc.NSString(action.Title), actionSelector(i), objc.NSString(action.Key))
		if item == 0 {
			return 0, fmt.Errorf("no menu item for %q", action.Title)
		}
		item.Send(objc.Sel("setTarget:"), target)
		menu.Send(objc.Sel("addItem:"), item)
	}
	return menu, nil
}

// actionSelector names the method that handles the action at index i. One
// selector per item keeps the mapping from menu item to Go function out of
// AppKit: neither the sender AppKit passes nor the item's tag has to be read
// back, and no two closures have to agree on a shared name.
func actionSelector(i int) objc.SEL {
	return objc.Sel(fmt.Sprintf("quadrantMenuAction%d:", i))
}

// QuadrantIcon returns the menu-bar image: a single-line square with a vertical
// and a horizontal line through its middle, dividing it into four quadrants.
//
// It is a template image, which is what makes it look right everywhere: AppKit
// draws a template itself -- white on a dark menu bar, black on a light one, and
// again whenever the menu bar's appearance changes. An image drawn in white
// would be invisible against a light menu bar.
func QuadrantIcon() (objc.ID, error) {
	icon, err := drawIcon(iconPoints, retina, func(size float64) {
		quadrants(size, iconInset, iconWeight, objc.ClassID("NSColor").Send(objc.Sel("blackColor")))
	})
	if err != nil {
		return 0, err
	}
	// From here on only the drawn alpha matters; the colour above has no say in
	// what the template is drawn in.
	icon.Send(objc.Sel("setTemplate:"), true)
	return icon, nil
}

// AboutIcon returns the image the About panel shows: the same mark in white on a
// rounded square in the system blue.
//
// The bundle carries the same art as an .icns, drawn by the same function
// (appIconArt, through AppIconPNG), so the panel, the Finder and the
// Accessibility pane cannot show three different icons. Replace them with a
// real icon the day there is one.
func AboutIcon() (objc.ID, error) {
	return drawIcon(aboutPoints, 1, appIconArt)
}

// appIconArt is the application's icon: the menu-bar mark in white on the
// rounded square macOS gives every application, at size points across.
//
// The blue is the fixed system blue rather than the user's accent colour
// (NSColor.controlAccentColor), which the menu bar icon gets away with and this
// does not: this art is also drawn once, as pixels, into the bundle's .icns, so
// a dynamic colour would be resolved against whatever state the process
// happened to be in at that moment -- an app that is not yet frontmost resolves
// the accent to graphite -- and then never change again, which is worse than
// not following the accent at all.
func appIconArt(size float64) {
	objc.ClassID("NSColor").Send(objc.Sel("systemBlueColor")).Send(objc.Sel("setFill"))
	// 0.2237 of the side is the corner radius of a macOS app icon.
	objc.ClassID("NSBezierPath").Send(objc.Sel("bezierPathWithRoundedRect:xRadius:yRadius:"),
		objc.NSRect{Size: objc.NSSize{Width: size, Height: size}}, size*0.2237, size*0.2237).
		Send(objc.Sel("fill"))
	quadrants(size, size*0.18, size/12, objc.ClassID("NSColor").Send(objc.Sel("whiteColor")))
}

// AboutPanel opens the standard About panel, the one every Mac application shows
// from its About item: the icon, the name, the version and the credits, with the
// version-copy behaviour and the layout AppKit already owns.
//
// The options are what an app with no bundle needs to pass. AppKit reads the
// name, version, credits and icon out of Info.plist, and this program is a bare
// executable with no Info.plist -- unbundled, the panel would show the process
// name and nothing else.
func AboutPanel(name, version, credits string, icon objc.ID) error {
	// App() loads AppKit and creates the shared NSApplication, which is the
	// object the panel belongs to and the one that is asked to come forward
	// below.
	app := objc.App()
	if app == 0 {
		return errors.New("no NSApplication")
	}
	options := objc.ClassID("NSMutableDictionary").Send(objc.Sel("alloc")).Send(objc.Sel("init"))
	if options == 0 {
		return errors.New("no about-panel options")
	}
	set := func(value objc.ID, key string) {
		if value != 0 {
			options.Send(objc.Sel("setObject:forKey:"), value, objc.NSString(key))
		}
	}
	set(objc.NSString(name), "ApplicationName")
	set(objc.NSString(version), "ApplicationVersion")
	set(attributedCredits(credits), "Credits")
	set(icon, "ApplicationIcon")

	// A process with the accessory activation policy is not frontmost, and a
	// panel it opens can stay behind whatever is. -activate is the macOS 14
	// spelling of this; earlier systems only have -activateIgnoringOtherApps:,
	// which has left the headers but not the runtime.
	if objc.Send[bool](app, objc.Sel("respondsToSelector:"), objc.Sel("activate")) {
		app.Send(objc.Sel("activate"))
	} else {
		app.Send(objc.Sel("activateIgnoringOtherApps:"), true)
	}
	app.Send(objc.Sel("orderFrontStandardAboutPanelWithOptions:"), options)
	return nil
}

// attributedCredits is the credits text in the form the About panel takes it: an
// attributed string, centre-aligned, in the system font at the size the panel's
// own credits are drawn at. The colour has to be given too -- the default is
// black, which is invisible on the dark panel dark mode draws. An empty text or
// a nil attribute yields 0, which AboutPanel leaves out of the options.
func attributedCredits(text string) objc.ID {
	if text == "" {
		return 0
	}
	paragraph := objc.ClassID("NSMutableParagraphStyle").Send(objc.Sel("alloc")).Send(objc.Sel("init"))
	if paragraph != 0 {
		paragraph.Send(objc.Sel("setAlignment:"), uint64(1)) // NSTextAlignmentCenter
	}
	font := objc.ClassID("NSFont").Send(objc.Sel("systemFontOfSize:"), float64(11))
	color := objc.ClassID("NSColor").Send(objc.Sel("secondaryLabelColor"))
	attributes := objc.ClassID("NSMutableDictionary").Send(objc.Sel("alloc")).Send(objc.Sel("init"))
	if attributes == 0 {
		return 0
	}
	for _, entry := range []struct {
		value objc.ID
		key   string
	}{
		{font, "NSFont"},
		{paragraph, "NSParagraphStyle"},
		{color, "NSForegroundColor"},
	} {
		if entry.value != 0 {
			attributes.Send(objc.Sel("setObject:forKey:"), entry.value, objc.NSString(entry.key))
		}
	}
	return objc.ClassID("NSAttributedString").Send(objc.Sel("alloc")).Send(
		objc.Sel("initWithString:attributes:"), objc.NSString(text), attributes)
}

// drawIcon draws a size-point image by running draw, backed by a bitmap scale
// times that many pixels across, and returns the image AppKit will show.
func drawIcon(size float64, scale int, draw func(size float64)) (objc.ID, error) {
	rep, err := bitmapRep(size, scale, draw)
	if err != nil {
		return 0, err
	}
	image := objc.ClassID("NSImage").Send(objc.Sel("alloc")).Send(objc.Sel("initWithSize:"), objc.NSSize{Width: size, Height: size})
	if image == 0 {
		return 0, errors.New("no image for the icon")
	}
	image.Send(objc.Sel("addRepresentation:"), rep)
	return image, nil
}

// bitmapRep draws size points at scale into a bitmap and returns the
// representation. Callers that want an image wrap it with drawIcon; the icon
// set generator wants the pixels themselves.
//
// The pixel count is explicit because that is what keeps the lines crisp: an
// 18pt icon drawn into an 18-pixel bitmap and shown on a retina display is
// enlarged from it, with the 1.5pt line smeared across three pixels rather than
// drawn as three. (lockFocus, the older way to draw into an NSImage, is
// deprecated and decides the scale itself; a bitmap this code allocates is also
// one this code can measure.)
func bitmapRep(size float64, scale int, draw func(size float64)) (objc.ID, error) {
	if err := objc.Load(objc.AppKit); err != nil {
		return 0, fmt.Errorf("load AppKit: %w", err)
	}
	pixels := int(size) * scale
	// AppKit zeroes the pages this initializer allocates, so the surface starts
	// transparent: the icon is the ink drawn below and nothing else.
	rep := objc.ClassID("NSBitmapImageRep").Send(objc.Sel("alloc")).Send(
		objc.Sel("initWithBitmapDataPlanes:pixelsWide:pixelsHigh:bitsPerSample:samplesPerPixel:hasAlpha:isPlanar:colorSpaceName:bytesPerRow:bitsPerPixel:"),
		uintptr(0), uintptr(pixels), uintptr(pixels), uintptr(8), uintptr(4), true, false,
		objc.NSString("NSDeviceRGBColorSpace"), uintptr(0), uintptr(0))
	if rep == 0 {
		return 0, fmt.Errorf("no %dx%d bitmap", pixels, pixels)
	}
	// The bitmap's size in points, which is what makes it a scale-times
	// representation rather than a huge image.
	rep.Send(objc.Sel("setSize:"), objc.NSSize{Width: size, Height: size})

	context := objc.ClassID("NSGraphicsContext").Send(objc.Sel("graphicsContextWithBitmapImageRep:"), rep)
	if context == 0 {
		return 0, errors.New("no drawing context for the icon")
	}
	objc.ClassID("NSGraphicsContext").Send(objc.Sel("saveGraphicsState"))
	objc.ClassID("NSGraphicsContext").Send(objc.Sel("setCurrentContext:"), context)
	draw(size)
	objc.ClassID("NSGraphicsContext").Send(objc.Sel("restoreGraphicsState"))
	return rep, nil
}

// quadrants strokes a square outline from inset to size-inset with a cross
// through its middle, weight points thick, in ink: four quadrants.
func quadrants(size, inset, weight float64, ink objc.ID) {
	ink.Send(objc.Sel("setStroke"))
	middle := size / 2

	box := objc.ClassID("NSBezierPath").Send(objc.Sel("bezierPathWithRect:"), objc.NSRect{
		Origin: objc.NSPoint{X: inset, Y: inset},
		Size:   objc.NSSize{Width: size - 2*inset, Height: size - 2*inset},
	})
	box.Send(objc.Sel("setLineWidth:"), weight)
	box.Send(objc.Sel("stroke"))

	cross := objc.ClassID("NSBezierPath").Send(objc.Sel("bezierPath"))
	cross.Send(objc.Sel("moveToPoint:"), objc.NSPoint{X: middle, Y: inset})
	cross.Send(objc.Sel("lineToPoint:"), objc.NSPoint{X: middle, Y: size - inset})
	cross.Send(objc.Sel("moveToPoint:"), objc.NSPoint{X: inset, Y: middle})
	cross.Send(objc.Sel("lineToPoint:"), objc.NSPoint{X: size - inset, Y: middle})
	cross.Send(objc.Sel("setLineWidth:"), weight)
	cross.Send(objc.Sel("stroke"))
}
