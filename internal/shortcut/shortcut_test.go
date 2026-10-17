package shortcut

import (
	"testing"

	"github.com/go-macos/hotkey"
)

func TestDefaultBindings(t *testing.T) {
	tests := []struct {
		input string
		key   hotkey.Key
	}{
		{"control+option+;", 0x29},
		{"control+option+'", 0x27},
		{"control+option+,", 0x2b},
		{"control+option+/", 0x2c},
	}
	for _, tc := range tests {
		got, err := Parse(tc.input)
		if err != nil || got.Key != tc.key || got.Mods != hotkey.Control|hotkey.Option {
			t.Errorf("%s: got %+v, %v", tc.input, got, err)
		}
	}
}

func TestArrowKeyBindings(t *testing.T) {
	tests := []struct {
		input string
		key   hotkey.Key
	}{
		{"control+option+left", hotkey.KeyLeftArrow},
		{"control+option+right", hotkey.KeyRightArrow},
		{"control+option+up", hotkey.KeyUpArrow},
		{"control+option+down", hotkey.KeyDownArrow},
		// The spelled-out aliases resolve to the same keys.
		{"control+option+leftarrow", hotkey.KeyLeftArrow},
		{"Control+Option+UpArrow", hotkey.KeyUpArrow},
	}
	for _, tc := range tests {
		got, err := Parse(tc.input)
		if err != nil || got.Key != tc.key || got.Mods != hotkey.Control|hotkey.Option {
			t.Errorf("%s: got %+v, %v; want key %v", tc.input, got, err, tc.key)
		}
	}
}

func TestBadBindings(t *testing.T) {
	for _, input := range []string{";", "ctrl+", "ctrl+alt+", "ctrl+wat+g", "ctrl+ctrl+;"} {
		if _, err := Parse(input); err == nil {
			t.Errorf("expected failure for %q", input)
		}
	}
}
