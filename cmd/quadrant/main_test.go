package main

import (
	"testing"

	"github.com/go-macos/hotkey"

	"quadrant/internal/config"
)

// parseBindings is the gate every configuration passes before a live hot key is
// touched, so what it refuses matters more than what it accepts.
func TestParseBindings(t *testing.T) {
	changed := func(change func(*config.Shortcuts)) config.Configuration {
		cfg := config.Default()
		change(&cfg.Shortcuts)
		return cfg
	}

	tests := []struct {
		name    string
		cfg     config.Configuration
		want    int
		wantErr bool
	}{
		{
			name: "the defaults bind all eight placements",
			cfg:  changed(func(*config.Shortcuts) {}),
			want: 8,
		},
		{
			name: "a configuration from before the halves were added",
			cfg: changed(func(s *config.Shortcuts) {
				s.LeftHalf, s.RightHalf, s.TopHalf, s.BottomHalf = "", "", "", ""
			}),
			want: 4,
		},
		{
			name: "an empty optional half is skipped, not registered",
			cfg:  changed(func(s *config.Shortcuts) { s.LeftHalf = "  " }),
			want: 7,
		},
		{
			name:    "one combination for two placements",
			cfg:     changed(func(s *config.Shortcuts) { s.TopRight = s.TopLeft }),
			wantErr: true,
		},
		{
			name:    "a key name nothing knows",
			cfg:     changed(func(s *config.Shortcuts) { s.TopLeft = "control+option+nosuchkey" }),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bindings, err := parseBindings(tc.cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseBindings accepted %+v, want an error", tc.cfg.Shortcuts)
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
