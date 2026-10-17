package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// watchInterval is far shorter than ReloadInterval so the tests finish quickly;
// Watch is meant to be indifferent to the interval it is given.
const watchInterval = 5 * time.Millisecond

// watchTimeout bounds every wait for the watcher to notice something. It is
// hundreds of intervals, so a slow machine does not turn into a flaky failure.
const watchTimeout = 5 * time.Second

// quietPolls is how long a test lets the watcher keep polling while asserting
// that nothing further happens.
const quietPolls = 20 * watchInterval

// lockedBuffer is a log destination the test goroutine can read while the
// watcher goroutine writes, without tripping the race detector.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLog redirects the standard logger for the test's duration and returns
// the buffer, so a test can assert on exactly what the user would see.
func captureLog(t *testing.T) *lockedBuffer {
	t.Helper()
	buf := &lockedBuffer{}
	previous := log.Writer()
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(previous) })
	return buf
}

// writeConfig writes a valid configuration with the given padding and returns
// the bytes, so a test can assert on what the watcher read.
func writeConfig(t *testing.T, path string, padding float64) []byte {
	t.Helper()
	cfg := Default()
	cfg.PaddingPoints = padding
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	return b
}

// startWatcher runs Watch with a short interval and a context cancelled when the
// test ends. It loads the file first, exactly as main does, so the watcher
// compares against the content that produced the configuration in service.
func startWatcher(t *testing.T, path string, apply func(Configuration) error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go Watch(ctx, path, watchInterval, loadedContent(t, path), apply)
}

// loadedContent is the content LoadOrCreate would report as being in service.
func loadedContent(t *testing.T, path string) []byte {
	t.Helper()
	_, content, err := LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

// waitFor waits for cond, or fails the test.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(watchTimeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// applyRecorder collects the configurations handed to apply.
type applyRecorder struct {
	applied chan Configuration
}

func newApplyRecorder() *applyRecorder {
	return &applyRecorder{applied: make(chan Configuration, 8)}
}

func (r *applyRecorder) apply(cfg Configuration) error {
	r.applied <- cfg
	return nil
}

// next returns the next applied configuration, or fails the test.
func (r *applyRecorder) next(t *testing.T) Configuration {
	t.Helper()
	select {
	case cfg := <-r.applied:
		return cfg
	case <-time.After(watchTimeout):
		t.Fatal("no configuration was applied")
		return Configuration{}
	}
}

// assertNone fails when anything was applied during d.
func (r *applyRecorder) assertNone(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case cfg := <-r.applied:
		t.Fatalf("unexpected reload: %+v", cfg)
	case <-time.After(d):
	}
}

func TestWatchReloadsChangedFile(t *testing.T) {
	buf := captureLog(t)
	path := filepath.Join(t.TempDir(), FileName)
	writeConfig(t, path, 0)

	rec := newApplyRecorder()
	startWatcher(t, path, rec.apply)

	writeConfig(t, path, 24)
	if got := rec.next(t).PaddingPoints; got != 24 {
		t.Fatalf("applied padding %v; want 24", got)
	}

	// A successful reload is logged once, and the polls around it stay silent.
	waitFor(t, "the reload to be logged", func() bool {
		return strings.Contains(buf.String(), "configuration reloaded: "+path)
	})
	time.Sleep(quietPolls)
	if lines := strings.Count(buf.String(), "\n"); lines != 1 {
		t.Fatalf("got %d log lines for one reload; want 1: %q", lines, buf.String())
	}
}

func TestWatchIgnoresUnchangedFile(t *testing.T) {
	buf := captureLog(t)
	path := filepath.Join(t.TempDir(), FileName)
	writeConfig(t, path, 8)

	rec := newApplyRecorder()
	startWatcher(t, path, rec.apply)

	rec.assertNone(t, quietPolls)
	if got := buf.String(); got != "" {
		t.Fatalf("polling an unchanged file logged %q; want nothing", got)
	}
}

// A file that does not validate is not applied, is reported once rather than on
// every poll, and is picked up again once it is fixed.
func TestWatchIgnoresInvalidFile(t *testing.T) {
	buf := captureLog(t)
	path := filepath.Join(t.TempDir(), FileName)
	writeConfig(t, path, 4)

	rec := newApplyRecorder()
	startWatcher(t, path, rec.apply)

	// Valid JSON, but the required top_left quadrant is missing.
	if err := os.WriteFile(path, []byte(`{"shortcuts":{"top_left":""}}`), 0600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the invalid file to be reported", func() bool {
		return strings.Contains(buf.String(), "not reloaded")
	})
	time.Sleep(quietPolls)
	if lines := strings.Count(buf.String(), "\n"); lines != 1 {
		t.Fatalf("got %d log lines for one bad file; want 1: %q", lines, buf.String())
	}
	// The invalid content was not applied, and the running configuration is
	// still whatever it was; the next read below is the first thing applied.

	writeConfig(t, path, 32)
	if got := rec.next(t).PaddingPoints; got != 32 {
		t.Fatalf("applied padding %v after the file was fixed; want 32", got)
	}
}

func TestWatchIgnoresDeletedFile(t *testing.T) {
	buf := captureLog(t)
	path := filepath.Join(t.TempDir(), FileName)
	writeConfig(t, path, 12)

	rec := newApplyRecorder()
	startWatcher(t, path, rec.apply)

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	rec.assertNone(t, quietPolls)
	if got := buf.String(); got != "" {
		t.Fatalf("a deleted file logged %q; want nothing", got)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the watcher recreated a deleted configuration")
	}
}

// apply can refuse a configuration the loader accepted - a shortcut another
// process has claimed, say. That refusal is reported once and is not a reload.
func TestWatchReportsFailedApplyOnce(t *testing.T) {
	buf := captureLog(t)
	path := filepath.Join(t.TempDir(), FileName)
	writeConfig(t, path, 0)

	apply := func(Configuration) error { return errors.New("combination already taken") }
	startWatcher(t, path, apply)

	writeConfig(t, path, 64)
	waitFor(t, "the failed reload to be reported", func() bool {
		return strings.Contains(buf.String(), "already taken")
	})
	time.Sleep(quietPolls)
	logged := buf.String()
	if lines := strings.Count(logged, "\n"); lines != 1 {
		t.Fatalf("got %d log lines for one failed reload; want 1: %q", lines, logged)
	}
	if strings.Contains(logged, "configuration reloaded") {
		t.Fatalf("a failed reload must not be reported as a reload: %q", logged)
	}
}

func TestWatchStopsWhenCancelled(t *testing.T) {
	buf := captureLog(t)
	path := filepath.Join(t.TempDir(), FileName)
	writeConfig(t, path, 0)

	rec := newApplyRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	go Watch(ctx, path, watchInterval, loadedContent(t, path), rec.apply)
	cancel()

	writeConfig(t, path, 99)
	rec.assertNone(t, quietPolls)
	if got := buf.String(); got != "" {
		t.Fatalf("a cancelled watcher logged %q; want nothing", got)
	}
}

// A change saved between the caller's load and the watcher's first poll must
// still take effect: the watcher compares against the content it was handed,
// not against whatever happens to be on disk when it starts.
func TestWatchAppliesChangeMadeBeforeFirstPoll(t *testing.T) {
	captureLog(t)
	path := filepath.Join(t.TempDir(), FileName)
	writeConfig(t, path, 0)
	inService := loadedContent(t, path)

	// The user saves a new value while the program is still starting up.
	writeConfig(t, path, 7)

	rec := newApplyRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Watch(ctx, path, watchInterval, inService, rec.apply)

	if got := rec.next(t).PaddingPoints; got != 7 {
		t.Fatalf("applied padding %v; want 7", got)
	}
}
