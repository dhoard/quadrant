// Package shortcut parses bindings such as "control+;", "command+shift+k",
// and "option+command+left_arrow" into Carbon virtual-key based hotkeys.
package shortcut

import (
	"fmt"
	"strings"

	"github.com/go-macos/hotkey"
)

// modifierIDs are the only accepted modifier spellings. There are deliberately
// no short forms or glyph aliases: a binding using ctrl, cmd, opt, alt, meta,
// ctl, or a modifier glyph is rejected rather than translated.
var modifierIDs = map[string]hotkey.Modifier{
	"control": hotkey.Control,
	"command": hotkey.Command,
	"shift":   hotkey.Shift,
	"option":  hotkey.Option,
}

// namedKeys are the only accepted named keys. Arrow names are multi-word and
// use an underscore; the short forms left/right/up/down are not accepted.
var namedKeys = map[string]hotkey.Key{
	"left_arrow":  hotkey.KeyLeftArrow,
	"right_arrow": hotkey.KeyRightArrow,
	"up_arrow":    hotkey.KeyUpArrow,
	"down_arrow":  hotkey.KeyDownArrow,

	"space":  hotkey.KeySpace,
	"return": hotkey.KeyReturn,
	"tab":    hotkey.KeyTab,
	"delete": hotkey.KeyDelete,
	"escape": hotkey.KeyEscape,

	"f1":  hotkey.KeyF1,
	"f2":  hotkey.KeyF2,
	"f3":  hotkey.KeyF3,
	"f4":  hotkey.KeyF4,
	"f5":  hotkey.KeyF5,
	"f6":  hotkey.KeyF6,
	"f7":  hotkey.KeyF7,
	"f8":  hotkey.KeyF8,
	"f9":  hotkey.KeyF9,
	"f10": hotkey.KeyF10,
	"f11": hotkey.KeyF11,
	"f12": hotkey.KeyF12,
	"f13": hotkey.KeyF13,
	"f14": hotkey.KeyF14,
	"f15": hotkey.KeyF15,
}

// keySpec is a key plus whether the character needs Shift on a US/ANSI
// keyboard. A character is a position plus modifiers, so "!" is the 1 key with
// Shift rather than a key code of its own.
type keySpec struct {
	key   hotkey.Key
	shift bool
}

// Virtual key codes for the punctuation the hotkey package does not name.
// These are the US/ANSI positions, which is the layout this configuration
// grammar describes.
const (
	keySemicolon  hotkey.Key = 0x29
	keyApostrophe hotkey.Key = 0x27
	keyComma      hotkey.Key = 0x2B
	keyPeriod     hotkey.Key = 0x2F
	keyBackslash  hotkey.Key = 0x2A
	keyGrave      hotkey.Key = 0x32
)

// symbolKeys maps literal symbols, and the three symbol names that are awkward
// to write in a binding, to their key positions. Shifted symbols carry
// shift=true; the parser adds Shift to the combination for them.
var symbolKeys = map[string]keySpec{
	";": {keySemicolon, false},
	":": {keySemicolon, true},

	"'":  {keyApostrophe, false},
	"\"": {keyApostrophe, true},

	",": {keyComma, false},
	"<": {keyComma, true},

	".": {keyPeriod, false},
	">": {keyPeriod, true},

	"/": {hotkey.KeySlash, false},
	"?": {hotkey.KeySlash, true},

	"\\": {keyBackslash, false},
	"|":  {keyBackslash, true},

	"-": {hotkey.KeyMinus, false},
	"_": {hotkey.KeyMinus, true},

	"=": {hotkey.KeyEqual, false},
	"+": {hotkey.KeyEqual, true},

	"[": {hotkey.KeyLeftBracket, false},
	"{": {hotkey.KeyLeftBracket, true},

	"]": {hotkey.KeyRightBracket, false},
	"}": {hotkey.KeyRightBracket, true},

	"`": {keyGrave, false},
	"~": {keyGrave, true},

	"!": {hotkey.KeyN1, true},
	"@": {hotkey.KeyN2, true},
	"#": {hotkey.KeyN3, true},
	"$": {hotkey.KeyN4, true},
	"%": {hotkey.KeyN5, true},
	"^": {hotkey.KeyN6, true},
	"&": {hotkey.KeyN7, true},
	"*": {hotkey.KeyN8, true},
	"(": {hotkey.KeyN9, true},
	")": {hotkey.KeyN0, true},

	"plus":        {hotkey.KeyEqual, true},
	"backslash":   {keyBackslash, false},
	"doublequote": {keyApostrophe, true},
}

// alnumKeys maps letters and digits to their ANSI positions. Letters are
// looked up after lowercasing, so a binding may write A or a; neither implies
// Shift.
var alnumKeys = func() map[string]hotkey.Key {
	m := map[string]hotkey.Key{
		"0": hotkey.KeyN0,
		"1": hotkey.KeyN1,
		"2": hotkey.KeyN2,
		"3": hotkey.KeyN3,
		"4": hotkey.KeyN4,
		"5": hotkey.KeyN5,
		"6": hotkey.KeyN6,
		"7": hotkey.KeyN7,
		"8": hotkey.KeyN8,
		"9": hotkey.KeyN9,
	}
	letters := []hotkey.Key{
		hotkey.KeyA, hotkey.KeyB, hotkey.KeyC, hotkey.KeyD, hotkey.KeyE,
		hotkey.KeyF, hotkey.KeyG, hotkey.KeyH, hotkey.KeyI, hotkey.KeyJ,
		hotkey.KeyK, hotkey.KeyL, hotkey.KeyM, hotkey.KeyN, hotkey.KeyO,
		hotkey.KeyP, hotkey.KeyQ, hotkey.KeyR, hotkey.KeyS, hotkey.KeyT,
		hotkey.KeyU, hotkey.KeyV, hotkey.KeyW, hotkey.KeyX, hotkey.KeyY,
		hotkey.KeyZ,
	}
	for i, key := range letters {
		m[string(rune('a'+i))] = key
	}
	return m
}()

// Parse requires exactly one key and at least one modifier. It accepts only
// the identifiers in modifierIDs and namedKeys plus literal letters, digits,
// and symbols; there are no aliases.
func Parse(text string) (hotkey.Combo, error) {
	rest := strings.TrimSpace(text)
	if rest == "" {
		return hotkey.Combo{}, fmt.Errorf("empty binding; expected modifiers+key")
	}

	var mods hotkey.Modifier
	for {
		token, tail, found := cutAtPlus(rest)
		if !found {
			break
		}
		name := strings.ToLower(strings.TrimSpace(token))
		bit, ok := modifierIDs[name]
		if !ok {
			break
		}
		if mods&bit != 0 {
			return hotkey.Combo{}, fmt.Errorf("%q: duplicate modifier %q", text, name)
		}
		mods |= bit
		rest = strings.TrimSpace(tail)
	}

	if mods == 0 {
		return hotkey.Combo{}, fmt.Errorf("%q: expected at least one modifier from control, command, shift, option", text)
	}

	keyText := strings.TrimSpace(rest)
	if keyText == "" {
		return hotkey.Combo{}, fmt.Errorf("%q: expected a key after the modifiers", text)
	}
	// A key is one token. A literal + is the one exception; it is handled by
	// symbolKeys, including its escaped form \+.
	if keyText != "+" && containsUnescapedPlus(keyText) {
		return hotkey.Combo{}, fmt.Errorf("%q: invalid modifier or key sequence %q", text, keyText)
	}

	spec, ok := lookupKey(keyText)
	if !ok {
		return hotkey.Combo{}, fmt.Errorf("%q: unknown key %q", text, keyText)
	}
	if spec.shift {
		mods |= hotkey.Shift
	}
	return hotkey.Combo{Key: spec.key, Mods: mods}, nil
}

// cutAtPlus cuts at the first + that is not escaped as \+.
func cutAtPlus(s string) (before, after string, found bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && s[i+1] == '+' {
			i++
			continue
		}
		if s[i] == '+' {
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}

// containsUnescapedPlus reports whether s has a + that is not escaped as \+.
func containsUnescapedPlus(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && s[i+1] == '+' {
			i++
			continue
		}
		if s[i] == '+' {
			return true
		}
	}
	return false
}

// lookupKey resolves one key token. Literal symbols are matched exactly;
// names and alphanumerics are matched case-insensitively.
func lookupKey(text string) (keySpec, bool) {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, `\+`, `+`)
	if text == "" {
		return keySpec{}, false
	}

	lower := strings.ToLower(text)
	if key, ok := namedKeys[lower]; ok {
		return keySpec{key: key}, true
	}
	if spec, ok := symbolKeys[text]; ok {
		return spec, true
	}
	if spec, ok := symbolKeys[lower]; ok {
		return spec, true
	}
	if len(lower) == 1 {
		if key, ok := alnumKeys[lower]; ok {
			return keySpec{key: key}, true
		}
	}
	return keySpec{}, false
}
