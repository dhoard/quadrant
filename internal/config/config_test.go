package config

import (
	"bytes"
	"encoding/json"
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
	if err != nil || !strings.Contains(string(b), "control+command+;") {
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
	content := []byte(`{"shortcuts":{"top_left":"control+command+;"}, "unknown":3}`)
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

func TestDefaultShortcutsUseControlCommand(t *testing.T) {
	cfg := Default()
	want := map[string]string{
		"top_left":     "control+command+;",
		"top_right":    "control+command+'",
		"bottom_left":  "control+command+,",
		"bottom_right": "control+command+/",
		"left_half":    "control+command+left_arrow",
		"right_half":   "control+command+right_arrow",
		"top_half":     "control+command+up_arrow",
		"bottom_half":  "control+command+down_arrow",
	}
	for _, entry := range cfg.Shortcuts.Entries() {
		if entry.Binding != want[entry.Name] {
			t.Errorf("%s: got %q; want %q", entry.Name, entry.Binding, want[entry.Name])
		}
	}
}

// The example configuration is documentation as well as a development input;
// keep it identical to the defaults rather than letting the two drift.
func TestExampleConfigMatchesDefault(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "configuration.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got Configuration
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("parse configuration.example.json: %v", err)
	}
	if got != Default() {
		t.Errorf("configuration.example.json = %+v; want %+v", got, Default())
	}
}

// The half-screen placements are optional; a configuration may omit them and
// they are simply left unregistered.
func TestConfigWithoutHalves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "configuration.json")
	content := []byte(`{"shortcuts":{"top_left":"control+command+;","top_right":"control+command+'","bottom_left":"control+command+,","bottom_right":"control+command+/"},"padding_points":0}`)
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadOrCreate(path)
	if err != nil {
		t.Fatalf("config without halves rejected: %v", err)
	}
	if cfg.Shortcuts.LeftHalf != "" || cfg.Shortcuts.BottomHalf != "" {
		t.Errorf("config without halves should leave halves empty, got %+v", cfg.Shortcuts)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(content) {
		t.Fatal("config without halves was modified")
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
