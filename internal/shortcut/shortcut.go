// Package shortcut parses bindings such as "control+;", "ctrl+shift+k",
// and "command+option+/" into Carbon virtual-key based hotkeys.
package shortcut

import (
	"fmt"
	"strings"

	"github.com/go-macos/hotkey"
)

var punctuation = map[string]hotkey.Key{
	";": 0x29, "semicolon": 0x29,
	"'": 0x27, "apostrophe": 0x27, "quote": 0x27,
	",": 0x2b, "comma": 0x2b,
	"/": hotkey.KeySlash, "slash": hotkey.KeySlash,
	".": 0x2f, "period": 0x2f, "dot": 0x2f,
	"-": hotkey.KeyMinus, "minus": hotkey.KeyMinus,
	"=": hotkey.KeyEqual, "equal": hotkey.KeyEqual,
	"[": hotkey.KeyLeftBracket, "leftbracket": hotkey.KeyLeftBracket,
	"]": hotkey.KeyRightBracket, "rightbracket": hotkey.KeyRightBracket,
}

// Parse requires exactly one non-modifier key and at least one modifier.
func Parse(text string) (hotkey.Combo, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(text)), "+")
	if len(parts) < 2 {
		return hotkey.Combo{}, fmt.Errorf("%q: expected modifiers+key (e.g. control+;)", text)
	}
	var mods hotkey.Modifier
	seen := map[string]bool{}
	for _, raw := range parts[:len(parts)-1] {
		p := strings.TrimSpace(raw)
		var name string
		switch p {
		case "ctrl", "control", "ctl", "⌃":
			name = "control"
			mods |= hotkey.Control
		case "alt", "option", "opt", "⌥":
			name = "option"
			mods |= hotkey.Option
		case "shift", "⇧":
			name = "shift"
			mods |= hotkey.Shift
		case "cmd", "command", "meta", "⌘":
			name = "command"
			mods |= hotkey.Command
		default:
			return hotkey.Combo{}, fmt.Errorf("%q: unknown modifier %q", text, p)
		}
		if seen[name] {
			return hotkey.Combo{}, fmt.Errorf("%q: duplicate modifier %s", text, p)
		}
		seen[name] = true
	}
	keyText := strings.TrimSpace(parts[len(parts)-1])
	key, ok := punctuation[keyText]
	if !ok {
		// KeyForChar maps letters and digits to the standard ANSI key code.
		key, ok = hotkey.KeyForChar(keyText)
	}
	if !ok {
		// The hotkey library supports named function/arrow keys too.
		fallback, err := hotkey.ParseCombo("Control+" + keyText)
		if err != nil {
			return hotkey.Combo{}, fmt.Errorf("%q: unknown key %q: %w", text, keyText, err)
		}
		key = fallback.Key
	}
	return hotkey.Combo{Key: key, Mods: mods}, nil
}
