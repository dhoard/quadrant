package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPathIsQuadrantDirectory(t *testing.T) {
	path, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != FileName {
		t.Errorf("file name: got %q; want %q", filepath.Base(path), FileName)
	}
	if dir := filepath.Base(filepath.Dir(path)); dir != ".quadrant" {
		t.Errorf("directory: got %q; want %q", dir, ".quadrant")
	}
}

func TestLoadOrCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "configuration.json")
	cfg, content, err := LoadOrCreate(path)
	if err != nil || cfg != Default() {
		t.Fatalf("got %+v, %v", cfg, err)
	}
	b, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(b), "control+option+;") {
		t.Fatalf("invalid saved config: %q, %v", b, err)
	}
	// The returned content is what the configuration was decoded from, which
	// is what Watch compares later polls against.
	if !bytes.Equal(content, b) {
		t.Fatalf("LoadOrCreate returned %q for a file containing %q", content, b)
	}
	cfg2, _, err := LoadOrCreate(path)
	if err != nil || cfg2 != cfg {
		t.Fatalf("reload: %+v, %v", cfg2, err)
	}
}

func TestInvalidConfigNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "configuration.json")
	content := []byte(`{"shortcuts":{"top_left":"control+option+;"}, "unknown":3}`)
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadOrCreate(path); err == nil {
		t.Fatal("expected config parsing failure")
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(content) {
		t.Fatal("invalid config was overwritten")
	}
}

func TestDefaultShortcutsUseControlOption(t *testing.T) {
	cfg := Default()
	want := map[string]string{
		"top_left":     "control+option+;",
		"top_right":    "control+option+'",
		"bottom_left":  "control+option+,",
		"bottom_right": "control+option+/",
		"left_half":    "control+option+left",
		"right_half":   "control+option+right",
		"top_half":     "control+option+up",
		"bottom_half":  "control+option+down",
	}
	for _, entry := range cfg.Shortcuts.Entries() {
		if entry.Binding != want[entry.Name] {
			t.Errorf("%s: got %q; want %q", entry.Name, entry.Binding, want[entry.Name])
		}
	}
}

// A configuration written before half-screen support has no half entries; it
// must still load, with those placements simply left unregistered.
func TestLegacyConfigWithoutHalves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "configuration.json")
	content := []byte(`{"shortcuts":{"top_left":"control+option+;","top_right":"control+option+'","bottom_left":"control+option+,","bottom_right":"control+option+/"},"padding_points":0}`)
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadOrCreate(path)
	if err != nil {
		t.Fatalf("legacy config rejected: %v", err)
	}
	if cfg.Shortcuts.LeftHalf != "" || cfg.Shortcuts.BottomHalf != "" {
		t.Errorf("legacy config should leave halves empty, got %+v", cfg.Shortcuts)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(content) {
		t.Fatal("legacy config was modified")
	}
}

func TestRequiredQuadrantStillRequired(t *testing.T) {
	cfg := Default()
	cfg.Shortcuts.TopLeft = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected empty quadrant to be rejected")
	}
	cfg = Default()
	cfg.Shortcuts.LeftHalf = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("empty optional half should be accepted: %v", err)
	}
}
