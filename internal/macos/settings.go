package macos

import (
	"errors"
	"fmt"

	"github.com/go-macos/objc"
)

// accessibilitySettingsURL is System Settings, opened at the Accessibility
// pane: the switch the Accessibility grant is turned on with.
//
// The URL scheme is the one AppKit itself uses -- System Settings declares
// x-apple.systempreferences in its Info.plist -- and the query is the legacy
// pane and anchor, which the Settings app still resolves. Nothing here is
// load-bearing if Apple moves it: openURL: answers false and the caller says
// the pane's name in words, which is what the log does.
const accessibilitySettingsURL = "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"

// OpenAccessibilitySettings brings System Settings up at the Accessibility
// pane, which is the same place the Accessibility permission dialog sends the
// user and the only place the grant can be given.
//
// It must be called on the process main thread, like everything else that talks
// to AppKit.
func OpenAccessibilitySettings() error {
	if err := objc.Load(objc.AppKit); err != nil {
		return fmt.Errorf("load AppKit: %w", err)
	}
	url := objc.ClassID("NSURL").Send(objc.Sel("URLWithString:"), objc.NSString(accessibilitySettingsURL))
	if url == 0 {
		return errors.New("no URL for the Accessibility pane")
	}
	workspace := objc.ClassID("NSWorkspace").Send(objc.Sel("sharedWorkspace"))
	if workspace == 0 {
		return errors.New("no workspace to open the pane in")
	}
	if !objc.Send[bool](workspace, objc.Sel("openURL:"), url) {
		return errors.New("System Settings did not take the Accessibility pane URL")
	}
	return nil
}
