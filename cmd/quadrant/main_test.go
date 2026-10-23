package main

import (
	"strings"
	"testing"

	"github.com/go-macos/hotkey"

	"quadrant/internal/config"
	"quadrant/internal/geometry"
)

func configWith(rows, columns int, shortcuts map[string]geometry.Region) config.Configuration {
	return config.Configuration{
		Rows:          rows,
		Columns:       columns,
		Shortcuts:     config.ShortcutMap(shortcuts),
		PaddingPoints: 0,
	}
}

// parseBindings is the gate every configuration passes before a live hot key is
// touched, so what it refuses matters more than what it accepts.
func TestParseBindings(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.Configuration
		want    int
		wantErr string
	}{
		{
			name: "the defaults bind all eight placements",
			cfg:  config.Default(),
			want: 8,
		},
		{
			name: "the supplied 4x4 layout",
			cfg: configWith(4, 4, map[string]geometry.Region{
				"control+command+1": {Row: 1, Column: 1, RowSpan: 2, ColumnSpan: 2},
				"control+command+2": {Row: 1, Column: 3, RowSpan: 2, ColumnSpan: 2},
			}),
			want: 2,
		},
		{
			name: "modifier order is not a distinct mapping",
			cfg: configWith(2, 2, map[string]geometry.Region{
				"control+command+1": {Row: 1, Column: 1, RowSpan: 1, ColumnSpan: 1},
				"command+control+1": {Row: 1, Column: 2, RowSpan: 1, ColumnSpan: 1},
			}),
			wantErr: "duplicates",
		},
		{
			name: "case is compared, then rejected as a physical duplicate",
			cfg: configWith(2, 2, map[string]geometry.Region{
				"control+command+A": {Row: 1, Column: 1, RowSpan: 1, ColumnSpan: 1},
				"control+command+a": {Row: 1, Column: 2, RowSpan: 1, ColumnSpan: 1},
			}),
			wantErr: "duplicates",
		},
		{
			name: "shifted-symbol spelling is a physical duplicate",
			cfg: configWith(2, 2, map[string]geometry.Region{
				"control+command+!":       {Row: 1, Column: 1, RowSpan: 1, ColumnSpan: 1},
				"control+command+shift+1": {Row: 1, Column: 2, RowSpan: 1, ColumnSpan: 1},
			}),
			wantErr: "duplicates",
		},
		{
			name: "two chords may deliberately share a region",
			cfg: configWith(2, 2, map[string]geometry.Region{
				"control+command+1": {Row: 1, Column: 1, RowSpan: 1, ColumnSpan: 1},
				"control+command+2": {Row: 1, Column: 1, RowSpan: 1, ColumnSpan: 1},
			}),
			want: 2,
		},
		{
			name: "literal plus uses plus",
			cfg: configWith(2, 2, map[string]geometry.Region{
				"control+command+plus": {Row: 1, Column: 1, RowSpan: 1, ColumnSpan: 1},
			}),
			want: 1,
		},
		{
			name: "escaped plus forms are rejected",
			cfg: configWith(2, 2, map[string]geometry.Region{
				"control+command++": {Row: 1, Column: 1, RowSpan: 1, ColumnSpan: 1},
			}),
			wantErr: "empty modifier or key",
		},
		{
			name: "key before modifier is rejected",
			cfg: configWith(2, 2, map[string]geometry.Region{
				"control+1+command": {Row: 1, Column: 1, RowSpan: 1, ColumnSpan: 1},
			}),
			wantErr: "unknown modifier",
		},
		{
			name: "a key name nothing knows",
			cfg: configWith(2, 2, map[string]geometry.Region{
				"control+command+nosuchkey": {Row: 1, Column: 1, RowSpan: 1, ColumnSpan: 1},
			}),
			wantErr: "unknown key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bindings, err := parseBindings(tc.cfg)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("parseBindings error = %v; want one containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseBindings: %v", err)
			}
			if len(bindings) != tc.want {
				t.Errorf("got %d bindings, want %d", len(bindings), tc.want)
			}
		})
	}
}

func TestParseBindingsCarriesGridRegionAndPadding(t *testing.T) {
	cfg := configWith(4, 4, map[string]geometry.Region{
		"control+command+2": {Row: 1, Column: 3, RowSpan: 2, ColumnSpan: 2},
		"control+command+1": {Row: 1, Column: 1, RowSpan: 2, ColumnSpan: 2},
	})
	cfg.PaddingPoints = 12

	bindings, err := parseBindings(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 2 {
		t.Fatalf("got %d bindings, want 2", len(bindings))
	}
	// Entries are sorted by their canonical comparison key.
	if bindings[0].label != "control+command+1" || bindings[1].label != "control+command+2" {
		t.Errorf("binding order = %q, %q", bindings[0].label, bindings[1].label)
	}
	want := geometry.Region{Row: 1, Column: 1, RowSpan: 2, ColumnSpan: 2}
	if bindings[0].grid != (geometry.Grid{Rows: 4, Columns: 4}) ||
		bindings[0].region != want ||
		bindings[0].padding != 12 {
		t.Errorf("first binding = %+v; want grid 4x4, region %+v, padding 12", bindings[0], want)
	}
}

// Without the Accessibility grant the shortcuts are not claimed -- a hot key
// that cannot move a window would answer every press with an error -- and the
// configuration is kept so that granting the permission later needs no restart.
func TestHotKeysWaitForTheGrant(t *testing.T) {
	svc := &service{opts: &hotkey.Options{}}
	cfg := config.Configuration{} // no bindings: no platform call is made

	if err := svc.apply(cfg); err != nil {
		t.Fatalf("apply without the grant: %v", err)
	}
	if svc.granted {
		t.Error("apply claimed the grant for itself")
	}
	if len(svc.keys) != 0 {
		t.Errorf("apply registered %d hot keys without the grant, want 0", len(svc.keys))
	}
	if svc.live.Load() == nil {
		t.Error("apply did not keep the configuration it was given")
	}

	if err := svc.grant(); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if !svc.granted {
		t.Error("grant did not record the grant")
	}
	// The watcher and a reload can race to call grant, so it is idempotent.
	if err := svc.grant(); err != nil {
		t.Fatalf("second grant: %v", err)
	}
	svc.close()
}
