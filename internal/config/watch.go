package config

import (
	"bytes"
	"context"
	"log"
	"os"
	"time"
)

// ReloadInterval is how often Watch re-reads the configuration file.
const ReloadInterval = 30 * time.Second

// Watch re-reads path every interval until ctx is cancelled, handing each
// newly loaded configuration to apply.
//
// current is the file content that produced the configuration currently in
// service — LoadOrCreate's second result. Comparing polls against those exact
// bytes, rather than re-reading the file to establish a baseline, is what makes
// a change saved while the program was starting up still take effect. A nil
// current means "nothing is in service yet", so the first poll applies whatever
// it finds.
//
// A poll is silent. Reloads are driven by the file's content, not its
// modification time: a poll that reads the same bytes as the previous poll does
// nothing at all, so a ticker that finds no change produces no output and the
// file is not re-parsed.
//
// When the bytes differ, they are parsed and validated with Parse. A file that
// fails to load or validate is NOT applied: the configuration currently in
// effect stays in place, exactly one warning is logged, and the rejected
// content is remembered so that a bad file left on disk is reported once rather
// than on every poll. It is retried only after the file changes again.
//
// apply receives every distinct configuration that loaded successfully and
// reports whether it could be put into service — for example, a shortcut may
// already be claimed by another process. An error from apply is logged once and
// shown to the user; the caller keeps whatever it had. Watch logs a single line
// per successful reload and nothing else.
//
// A deleted or unreadable file is not a change: the watcher leaves the running
// configuration alone instead of calling LoadOrCreate, which would recreate the
// file with the defaults. That way a configuration file replaced by an editor
// that writes-then-renames is never mistaken for a request to reset everything.
func Watch(ctx context.Context, path string, interval time.Duration, current []byte, apply func(Configuration) error) {
	if interval <= 0 {
		interval = ReloadInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	observed := current

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		// A tick that races with cancellation does no work: shutting down has
		// already begun and the hot keys are about to be released.
		if ctx.Err() != nil {
			return
		}

		content, err := os.ReadFile(path)
		if err != nil {
			// Absent or unreadable. Leave the running configuration alone rather
			// than calling LoadOrCreate, which would recreate a deleted file with
			// the defaults and reset everything.
			continue
		}
		if bytes.Equal(content, observed) {
			continue
		}
		// Parse the bytes just compared, not the file again: the content that
		// is validated, applied and remembered is one and the same, a change
		// saved after this read is seen by the next poll, and a file removed
		// in between cannot be recreated with the defaults.
		observed = content

		cfg, err := Parse(path, content)
		if err != nil {
			log.Printf("configuration not reloaded: %v (keeping the running configuration)", err)
			continue
		}
		if err := apply(cfg); err != nil {
			log.Printf("configuration not reloaded: %v (keeping the running configuration)", err)
			continue
		}
		log.Printf("configuration reloaded: %s", path)
	}
}
