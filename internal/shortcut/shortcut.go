// Package shortcut parses bindings such as "control+;", "command+shift+k",
// and "option+command+left_arrow" into Carbon virtual-key based hotkeys.
package shortcut

import (
	"fmt"
	"sort"
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
// and symbols; there are no aliases. `+` separates the tokens, so the literal
// plus key is written `plus`.
func Parse(text string) (hotkey.Combo, error) {
	tokens, err := splitBinding(text)
	if err != nil {
		return hotkey.Combo{}, err
	}
	if len(tokens) < 2 {
		return hotkey.Combo{}, fmt.Errorf("%q: expected at least one modifier and one key", text)
	}
	mods, err := parseModifiers(tokens[:len(tokens)-1], text)
	if err != nil {
		return hotkey.Combo{}, err
	}
	keyText := tokens[len(tokens)-1]
	spec, ok := lookupKey(keyText)
	if !ok {
		return hotkey.Combo{}, fmt.Errorf("%q: unknown key %q", text, keyText)
	}
	if spec.shift {
		mods |= hotkey.Shift
	}
	return hotkey.Combo{Key: spec.key, Mods: mods}, nil
}

// ComparisonKey returns a canonical form of a binding for case-sensitive map
// comparison. Modifier tokens are ordered canonically, but their text and the
// key token keep their original case. The key token must be last; only
// modifier tokens may be permuted.
func ComparisonKey(text string) (string, error) {
	tokens, err := splitBinding(text)
	if err != nil {
		return "", err
	}
	if len(tokens) < 2 {
		return "", fmt.Errorf("%q: expected at least one modifier and one key", text)
	}

	type modifierToken struct {
		bit  hotkey.Modifier
		text string
	}
	mods := make([]modifierToken, 0, len(tokens)-1)
	var seen hotkey.Modifier
	for _, token := range tokens[:len(tokens)-1] {
		bit, ok := modifierIDs[strings.ToLower(token)]
		if !ok {
			return "", fmt.Errorf("%q: unknown modifier %q", text, token)
		}
		if seen&bit != 0 {
			return "", fmt.Errorf("%q: duplicate modifier %q", text, token)
		}
		seen |= bit
		mods = append(mods, modifierToken{bit: bit, text: token})
	}
	sort.SliceStable(mods, func(i, j int) bool {
		return modifierRank(mods[i].bit) < modifierRank(mods[j].bit)
	})

	keyText := tokens[len(tokens)-1]
	if _, ok := lookupKey(keyText); !ok {
		return "", fmt.Errorf("%q: unknown key %q", text, keyText)
	}
	parts := make([]string, 0, len(mods)+1)
	for _, mod := range mods {
		parts = append(parts, mod.text)
	}
	parts = append(parts, keyText)
	return strings.Join(parts, "+"), nil
}

// splitBinding splits on the separator and rejects empty tokens. This is what
// rejects the old ++ and \+ literal-plus spellings: `plus` is the only literal
// plus key.
func splitBinding(text string) ([]string, error) {
	rest := strings.TrimSpace(text)
	if rest == "" {
		return nil, fmt.Errorf("empty binding; expected modifiers+key")
	}
	parts := strings.Split(rest, "+")
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("%q: empty modifier or key", text)
		}
		parts[i] = part
	}
	return parts, nil
}

func parseModifiers(tokens []string, text string) (hotkey.Modifier, error) {
	var mods hotkey.Modifier
	for _, token := range tokens {
		name := strings.ToLower(token)
		bit, ok := modifierIDs[name]
		if !ok {
			return 0, fmt.Errorf("%q: unknown modifier %q", text, token)
		}
		if mods&bit != 0 {
			return 0, fmt.Errorf("%q: duplicate modifier %q", text, name)
		}
		mods |= bit
	}
	if mods == 0 {
		return 0, fmt.Errorf("%q: expected at least one modifier from control, command, shift, option", text)
	}
	return mods, nil
}

// modifierRank gives the canonical reporting order: control, command, shift,
// option.
func modifierRank(bit hotkey.Modifier) int {
	switch bit {
	case hotkey.Control:
		return 0
	case hotkey.Command:
		return 1
	case hotkey.Shift:
		return 2
	case hotkey.Option:
		return 3
	default:
		return 4
	}
}

// lookupKey resolves one key token. Literal symbols are matched exactly;
// names and alphanumerics are matched case-insensitively.
func lookupKey(text string) (keySpec, bool) {
	text = strings.TrimSpace(text)
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
