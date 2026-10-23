package config

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"quadrant/internal/geometry"
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
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, Default()) {
		t.Fatalf("got %+v; want %+v", cfg, Default())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`"rows": 2`)) || !bytes.Contains(b, []byte("control+command+;")) {
		t.Fatalf("invalid saved config: %q", b)
	}
	// The returned content is what the configuration was decoded from, which
	// is what Watch compares later polls against.
	if !bytes.Equal(content, b) {
		t.Fatalf("LoadOrCreate returned %q for a file containing %q", content, b)
	}
	cfg2, _, err := LoadOrCreate(path)
	if err != nil || !reflect.DeepEqual(cfg2, cfg) {
		t.Fatalf("reload: %+v, %v", cfg2, err)
	}
}

func TestInvalidConfigNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "configuration.json")
	content := []byte(`{"rows":2,"columns":2,"shortcuts":{"control+command+;":{"row":1,"column":1}},"unknown":3}`)
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

func TestDefaultConfig(t *testing.T) {
	cfg := Default()
	if cfg.Rows != 2 || cfg.Columns != 2 {
		t.Fatalf("default grid = %dx%d; want 2x2", cfg.Rows, cfg.Columns)
	}
	want := map[string]geometry.Region{
		"control+command+;":           {Row: 1, Column: 1, RowSpan: 1, ColumnSpan: 1},
		"control+command+'":           {Row: 1, Column: 2, RowSpan: 1, ColumnSpan: 1},
		"control+command+.":           {Row: 2, Column: 1, RowSpan: 1, ColumnSpan: 1},
		"control+command+/":           {Row: 2, Column: 2, RowSpan: 1, ColumnSpan: 1},
		"control+command+left_arrow":  {Row: 1, Column: 1, RowSpan: 2, ColumnSpan: 1},
		"control+command+right_arrow": {Row: 1, Column: 2, RowSpan: 2, ColumnSpan: 1},
		"control+command+up_arrow":    {Row: 1, Column: 1, RowSpan: 1, ColumnSpan: 2},
		"control+command+down_arrow":  {Row: 2, Column: 1, RowSpan: 1, ColumnSpan: 2},
	}
	if !reflect.DeepEqual(map[string]geometry.Region(cfg.Shortcuts), want) {
		t.Errorf("default shortcuts = %+v; want %+v", cfg.Shortcuts, want)
	}
	if cfg.PaddingPoints != 0 {
		t.Errorf("default padding = %v; want 0", cfg.PaddingPoints)
	}

	// Default must not hand out a shared map that a caller can mutate.
	cfg.Shortcuts["control+command+1"] = geometry.Region{Row: 1, Column: 1, RowSpan: 1, ColumnSpan: 1}
	if _, found := Default().Shortcuts["control+command+1"]; found {
		t.Error("Default returned a shared shortcut map")
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
	if !reflect.DeepEqual(got, Default()) {
		t.Errorf("configuration.example.json = %+v; want %+v", got, Default())
	}
}

func TestDecodeGridAndSpans(t *testing.T) {
	path := filepath.Join(t.TempDir(), "configuration.json")
	content := []byte(`{
	  "rows": 4,
	  "columns": 4,
	  "shortcuts": {
	    "control+command+1": {"row": 1, "column": 1, "row_span": 2, "column_span": 2},
	    "control+command+2": {"row": 1, "column": 3, "row_span": 2, "column_span": 2}
	  },
	  "padding_points": 0
	}`)
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Rows != 4 || cfg.Columns != 4 {
		t.Errorf("grid = %dx%d; want 4x4", cfg.Rows, cfg.Columns)
	}
	if got := cfg.Shortcuts["control+command+1"]; got != (geometry.Region{Row: 1, Column: 1, RowSpan: 2, ColumnSpan: 2}) {
		t.Errorf("first region = %+v", got)
	}
	if got := cfg.Shortcuts["control+command+2"]; got != (geometry.Region{Row: 1, Column: 3, RowSpan: 2, ColumnSpan: 2}) {
		t.Errorf("second region = %+v", got)
	}
}

func TestSpansDefaultToOne(t *testing.T) {
	cfg := loadJSON(t, `{"rows":2,"columns":2,"shortcuts":{"control+command+;":{"row":1,"column":2}},"padding_points":0}`)
	got := cfg.Shortcuts["control+command+;"]
	if got != (geometry.Region{Row: 1, Column: 2, RowSpan: 1, ColumnSpan: 1}) {
		t.Errorf("region = %+v; want spans defaulted to 1", got)
	}
}

func TestJSONEntryOrderInsignificant(t *testing.T) {
	a := loadJSON(t, `{"rows":2,"columns":2,"shortcuts":{"control+command+;":{"row":1,"column":1},"control+command+.":{"row":2,"column":1}},"padding_points":0}`)
	b := loadJSON(t, `{"rows":2,"columns":2,"shortcuts":{"control+command+.":{"row":2,"column":1},"control+command+;":{"row":1,"column":1}},"padding_points":0}`)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("entry order changed the configuration: %+v vs %+v", a, b)
	}
}

func TestDuplicateJSONKeysRejected(t *testing.T) {
	path := writeTemp(t, `{"rows":2,"columns":2,"shortcuts":{"control+command+;":{"row":1,"column":1},"control+command+;":{"row":1,"column":2}},"padding_points":0}`)
	if _, _, err := LoadOrCreate(path); err == nil || !strings.Contains(err.Error(), "duplicate shortcut") {
		t.Fatalf("duplicate key error = %v", err)
	}
}

func TestUnknownRegionFieldRejected(t *testing.T) {
	path := writeTemp(t, `{"rows":2,"columns":2,"shortcuts":{"control+command+;":{"row":1,"column":1,"rowspan":2}},"padding_points":0}`)
	if _, _, err := LoadOrCreate(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown region field error = %v", err)
	}
}

func TestMissingGridDimensionsRejected(t *testing.T) {
	for _, content := range []string{
		`{"columns":2,"shortcuts":{"control+command+;":{"row":1,"column":1}},"padding_points":0}`,
		`{"rows":2,"shortcuts":{"control+command+;":{"row":1,"column":1}},"padding_points":0}`,
	} {
		path := writeTemp(t, content)
		if _, _, err := LoadOrCreate(path); err == nil {
			t.Errorf("accepted missing grid dimension in %s", content)
		}
	}
}

func TestShortcutsMustBeObject(t *testing.T) {
	for _, content := range []string{
		`{"rows":2,"columns":2,"shortcuts":[],"padding_points":0}`,
		`{"rows":2,"columns":2,"shortcuts":null,"padding_points":0}`,
	} {
		path := writeTemp(t, content)
		if _, _, err := LoadOrCreate(path); err == nil {
			t.Errorf("accepted shortcuts value in %s", content)
		}
	}
}

func TestOldFormatRejectedWithHint(t *testing.T) {
	path := writeTemp(t, `{"shortcuts":{"top_left":"control+command+;"},"padding_points":0}`)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = LoadOrCreate(path)
	if err == nil {
		t.Fatal("old format was accepted")
	}
	if !strings.Contains(err.Error(), "rows") || !strings.Contains(err.Error(), "columns") {
		t.Errorf("old-format error does not name the new format: %v", err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Error("old-format file was modified")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Configuration)
		wantOK bool
	}{
		{"default", func(*Configuration) {}, true},
		{"rows missing", func(c *Configuration) { c.Rows = 0 }, false},
		{"columns missing", func(c *Configuration) { c.Columns = 0 }, false},
		{"negative rows", func(c *Configuration) { c.Rows = -1 }, false},
		{"no shortcuts", func(c *Configuration) { c.Shortcuts = ShortcutMap{} }, false},
		{"padding negative", func(c *Configuration) { c.PaddingPoints = -1 }, false},
		{"padding too large", func(c *Configuration) { c.PaddingPoints = 129 }, false},
		{"padding NaN", func(c *Configuration) { c.PaddingPoints = math.NaN() }, false},
		{"row zero", func(c *Configuration) {
			c.Shortcuts = ShortcutMap{"control+command+1": {Row: 0, Column: 1, RowSpan: 1, ColumnSpan: 1}}
		}, false},
		{"row too large", func(c *Configuration) {
			c.Shortcuts = ShortcutMap{"control+command+1": {Row: 3, Column: 1, RowSpan: 1, ColumnSpan: 1}}
		}, false},
		{"column too large", func(c *Configuration) {
			c.Shortcuts = ShortcutMap{"control+command+1": {Row: 1, Column: 3, RowSpan: 1, ColumnSpan: 1}}
		}, false},
		{"row span zero", func(c *Configuration) {
			c.Shortcuts = ShortcutMap{"control+command+1": {Row: 1, Column: 1, RowSpan: 0, ColumnSpan: 1}}
		}, false},
		{"column span zero", func(c *Configuration) {
			c.Shortcuts = ShortcutMap{"control+command+1": {Row: 1, Column: 1, RowSpan: 1, ColumnSpan: 0}}
		}, false},
		{"row span exceeds grid", func(c *Configuration) {
			c.Shortcuts = ShortcutMap{"control+command+1": {Row: 2, Column: 1, RowSpan: 2, ColumnSpan: 1}}
		}, false},
		{"column span exceeds grid", func(c *Configuration) {
			c.Shortcuts = ShortcutMap{"control+command+1": {Row: 1, Column: 2, RowSpan: 1, ColumnSpan: 2}}
		}, false},
		{"empty binding key", func(c *Configuration) {
			c.Shortcuts = ShortcutMap{"  ": {Row: 1, Column: 1, RowSpan: 1, ColumnSpan: 1}}
		}, false},
		{"full grid span", func(c *Configuration) {
			c.Shortcuts = ShortcutMap{"control+command+1": {Row: 1, Column: 1, RowSpan: 2, ColumnSpan: 2}}
		}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			tc.change(&cfg)
			err := cfg.Validate()
			if tc.wantOK && err != nil {
				t.Errorf("Validate: %v", err)
			}
			if !tc.wantOK && err == nil {
				t.Errorf("Validate accepted %+v", cfg)
			}
		})
	}
}

func loadJSON(t *testing.T, content string) Configuration {
	t.Helper()
	path := writeTemp(t, content)
	cfg, _, err := LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
