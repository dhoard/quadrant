// Package grant waits for the macOS Accessibility grant to appear while the
// program keeps running.
//
// macOS hands the grant to the application that asks for it, and the person who
// grants it does so in System Settings, in their own time. Asking again every
// few seconds would mean a dialog every few seconds, and macOS tells a process
// nothing when the switch moves -- so the only way to start working before the
// next restart is to poll the state that costs nothing to read. That is the
// distinction this package is built on: the prompting variant of the check is
// called once, by the caller, and the silent variant is what Watch polls.
//
// Nothing here touches the Accessibility API itself, which is why it is
// testable everywhere the rest of the program is not.
package grant

import (
	"context"
	"time"
)

// Watch calls gained once, as soon as poll reports true, and then returns.
//
// poll is called once immediately and then every interval; it must be the
// silent check, never one that shows a dialog. A context that is already done
// returns without polling at all, so the shutdown path never registers hot keys
// in its last moments. interval must be positive, and gained runs on the
// caller's goroutine.
func Watch(ctx context.Context, interval time.Duration, poll func() bool, gained func()) {
	select {
	case <-ctx.Done():
		return
	default:
	}
	if poll() {
		gained()
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if poll() {
				gained()
				return
			}
		}
	}
}
