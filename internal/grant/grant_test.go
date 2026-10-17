package grant

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// A poll that is already true must not wait for the first interval: the program
// is trusted the moment it starts, and the shortcuts should appear then.
func TestWatchGainsImmediately(t *testing.T) {
	gained := 0
	Watch(context.Background(), time.Hour, func() bool { return true }, func() { gained++ })
	if gained != 1 {
		t.Fatalf("gained called %d times, want 1", gained)
	}
}

// The grant usually arrives after the program has been running for a while.
func TestWatchPollsUntilTheGrantAppears(t *testing.T) {
	var polls int32
	gained := make(chan struct{}, 1)
	Watch(context.Background(), time.Millisecond, func() bool {
		return atomic.AddInt32(&polls, 1) >= 3
	}, func() { gained <- struct{}{} })

	select {
	case <-gained:
	default:
		t.Fatal("gained was not called after poll reported the grant")
	}
	if got := atomic.LoadInt32(&polls); got != 3 {
		t.Errorf("poll called %d times, want 3 (Watch kept polling after gaining)", got)
	}
}

// Shutting down must not leave the poll running, and must not call gained on
// the way out.
func TestWatchStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var polls int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		Watch(ctx, time.Millisecond, func() bool {
			atomic.AddInt32(&polls, 1)
			return false
		}, func() { t.Error("gained called after the context was cancelled") })
	}()

	time.Sleep(5 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Watch did not return after its context was cancelled")
	}
	if atomic.LoadInt32(&polls) == 0 {
		t.Error("poll was never called")
	}
}

// A context that is already done returns before the first poll, so a grant that
// appears during shutdown cannot register hot keys in their last moments.
func TestWatchDoesNothingWithADoneContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	gained := 0
	polls := 0
	Watch(ctx, time.Millisecond, func() bool { polls++; return true }, func() { gained++ })
	if gained != 0 {
		t.Errorf("gained called %d times with a cancelled context, want 0", gained)
	}
}
