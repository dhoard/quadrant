package macos

import (
	"fmt"

	"github.com/go-macos/accessibility"
	"github.com/go-macos/objc"
)

// VisibleArea returns the selected display's NSScreen.visibleFrame expressed
// in CoreGraphics/AX global coordinates. It excludes Dock and menu-bar space.
// The AppKit query executes on the macOS main thread via DispatchMain.
func VisibleArea(display accessibility.Display) (accessibility.Rect, error) {
	type response struct {
		area accessibility.Rect
		err  error
	}
	result := make(chan response, 1)
	objc.DispatchMain(func() {
		var r response
		objc.AutoreleasePool(func() { r.area, r.err = visibleAreaOnMain(display) })
		result <- r
	})
	r := <-result
	return r.area, r.err
}

func visibleAreaOnMain(display accessibility.Display) (accessibility.Rect, error) {
	if err := objc.Load(objc.AppKit); err != nil {
		return accessibility.Rect{}, fmt.Errorf("load AppKit: %w", err)
	}
	screens := objc.ClassID("NSScreen").Send(objc.Sel("screens"))
	if screens == 0 {
		return accessibility.Rect{}, fmt.Errorf("AppKit returned no screens")
	}
	count := objc.Send[uint64](screens, objc.Sel("count"))
	if count == 0 {
		return accessibility.Rect{}, fmt.Errorf("no active displays")
	}

	// screens[0] is the primary/menu-bar display. AppKit uses bottom-left
	// coordinates; CG/AX uses the top-left of the primary display as (0,0).
	primary := screens.Send(objc.Sel("objectAtIndex:"), uint64(0))
	primaryFrame := objc.Send[objc.NSRect](primary, objc.Sel("frame"))
	primaryTop := primaryFrame.Origin.Y + primaryFrame.Size.Height

	for i := uint64(0); i < count; i++ {
		screen := screens.Send(objc.Sel("objectAtIndex:"), i)
		description := screen.Send(objc.Sel("deviceDescription"))
		idNumber := description.Send(objc.Sel("objectForKey:"), objc.NSString("NSScreenNumber"))
		if idNumber == 0 {
			continue
		}
		id := objc.Send[uint32](idNumber, objc.Sel("unsignedIntValue"))
		if id != display.ID {
			continue
		}
		visible := objc.Send[objc.NSRect](screen, objc.Sel("visibleFrame"))
		r := accessibility.Rect{
			X: visible.Origin.X - primaryFrame.Origin.X,
			Y: primaryTop - (visible.Origin.Y + visible.Size.Height),
			W: visible.Size.Width,
			H: visible.Size.Height,
		}
		if r.W <= 0 || r.H <= 0 {
			return accessibility.Rect{}, fmt.Errorf("invalid usable rectangle: %v", r)
		}
		return r, nil
	}
	return accessibility.Rect{}, fmt.Errorf("display %d not listed by NSScreen", display.ID)
}
