package shortcut

import (
	"testing"

	"github.com/go-macos/hotkey"
)

func mustParse(t *testing.T, input string) hotkey.Combo {
	t.Helper()
	got, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse(%q): %v", input, err)
	}
	return got
}

func TestDefaultBindings(t *testing.T) {
	tests := []struct {
		input string
		key   hotkey.Key
	}{
		{"control+command+;", keySemicolon},
		{"control+command+'", keyApostrophe},
		{"control+command+,", keyComma},
		{"control+command+/", hotkey.KeySlash},
	}
	wantMods := hotkey.Control | hotkey.Command
	for _, tc := range tests {
		got := mustParse(t, tc.input)
		if got.Key != tc.key || got.Mods != wantMods {
			t.Errorf("%s: got %+v; want key %v, mods %v", tc.input, got, tc.key, wantMods)
		}
	}
}

func TestArrowKeyBindings(t *testing.T) {
	tests := []struct {
		input string
		key   hotkey.Key
	}{
		{"control+command+left_arrow", hotkey.KeyLeftArrow},
		{"control+command+right_arrow", hotkey.KeyRightArrow},
		{"control+command+up_arrow", hotkey.KeyUpArrow},
		{"control+command+down_arrow", hotkey.KeyDownArrow},
		// Identifiers are case-insensitive.
		{"Control+Command+LEFT_ARROW", hotkey.KeyLeftArrow},
		{"CONTROL+COMMAND+Right_Arrow", hotkey.KeyRightArrow},
	}
	wantMods := hotkey.Control | hotkey.Command
	for _, tc := range tests {
		got := mustParse(t, tc.input)
		if got.Key != tc.key || got.Mods != wantMods {
			t.Errorf("%s: got %+v; want key %v, mods %v", tc.input, got, tc.key, wantMods)
		}
	}
}

func TestModifierIDs(t *testing.T) {
	tests := []struct {
		input string
		mods  hotkey.Modifier
	}{
		{"control+a", hotkey.Control},
		{"command+a", hotkey.Command},
		{"shift+a", hotkey.Shift},
		{"option+a", hotkey.Option},
		{"control+command+shift+option+a", hotkey.Control | hotkey.Command | hotkey.Shift | hotkey.Option},
		{"SHIFT+OPTION+a", hotkey.Shift | hotkey.Option},
		{"control+option+a", hotkey.Control | hotkey.Option},
	}
	for _, tc := range tests {
		got := mustParse(t, tc.input)
		if got.Key != hotkey.KeyA || got.Mods != tc.mods {
			t.Errorf("%s: got %+v; want KeyA, mods %v", tc.input, got, tc.mods)
		}
	}
}

func TestAlphaCaseAndDigits(t *testing.T) {
	lower := mustParse(t, "control+command+a")
	upper := mustParse(t, "control+command+A")
	if lower != upper {
		t.Errorf("letter case changed the combo: %+v vs %+v", lower, upper)
	}
	if lower.Mods&hotkey.Shift != 0 {
		t.Errorf("uppercase letter implied Shift: %+v", lower)
	}

	shifted := mustParse(t, "control+command+shift+a")
	if shifted.Mods&hotkey.Shift == 0 {
		t.Errorf("explicit shift missing: %+v", shifted)
	}
	if shifted == lower {
		t.Errorf("shift+a and a resolved to the same combo")
	}

	digits := []struct {
		input string
		key   hotkey.Key
	}{
		{"control+command+0", hotkey.KeyN0},
		{"control+command+1", hotkey.KeyN1},
		{"control+command+5", hotkey.KeyN5},
		{"control+command+9", hotkey.KeyN9},
	}
	for _, tc := range digits {
		got := mustParse(t, tc.input)
		if got.Key != tc.key || got.Mods != hotkey.Control|hotkey.Command {
			t.Errorf("%s: got %+v; want key %v", tc.input, got, tc.key)
		}
	}
}

func TestSymbols(t *testing.T) {
	tests := []struct {
		input string
		key   hotkey.Key
		shift bool
	}{
		{"control+command+;", keySemicolon, false},
		{"control+command+:", keySemicolon, true},
		{"control+command+'", keyApostrophe, false},
		{"control+command+\"", keyApostrophe, true},
		{"control+command+,", keyComma, false},
		{"control+command+<", keyComma, true},
		{"control+command+.", keyPeriod, false},
		{"control+command+>", keyPeriod, true},
		{"control+command+/", hotkey.KeySlash, false},
		{"control+command+?", hotkey.KeySlash, true},
		{"control+command+\\", keyBackslash, false},
		{"control+command+|", keyBackslash, true},
		{"control+command+-", hotkey.KeyMinus, false},
		{"control+command+_", hotkey.KeyMinus, true},
		{"control+command+=", hotkey.KeyEqual, false},
		{"control+command++", hotkey.KeyEqual, true},
		{"control+command+[", hotkey.KeyLeftBracket, false},
		{"control+command+{", hotkey.KeyLeftBracket, true},
		{"control+command+]", hotkey.KeyRightBracket, false},
		{"control+command+}", hotkey.KeyRightBracket, true},
		{"control+command+\x60", keyGrave, false},
		{"control+command+~", keyGrave, true},
		{"control+command+!", hotkey.KeyN1, true},
		{"control+command+@", hotkey.KeyN2, true},
		{"control+command+#", hotkey.KeyN3, true},
		{"control+command+$", hotkey.KeyN4, true},
		{"control+command+%", hotkey.KeyN5, true},
		{"control+command+^", hotkey.KeyN6, true},
		{"control+command+&", hotkey.KeyN7, true},
		{"control+command+*", hotkey.KeyN8, true},
		{"control+command+(", hotkey.KeyN9, true},
		{"control+command+)", hotkey.KeyN0, true},
		{"control+command+plus", hotkey.KeyEqual, true},
		{"control+command+backslash", keyBackslash, false},
		{"control+command+doublequote", keyApostrophe, true},
	}
	for _, tc := range tests {
		got := mustParse(t, tc.input)
		wantMods := hotkey.Control | hotkey.Command
		if tc.shift {
			wantMods |= hotkey.Shift
		}
		if got.Key != tc.key || got.Mods != wantMods {
			t.Errorf("%s: got %+v; want key %v, mods %v", tc.input, got, tc.key, wantMods)
		}
	}
}

func TestLiteralPlus(t *testing.T) {
	want := hotkey.Combo{Key: hotkey.KeyEqual, Mods: hotkey.Control | hotkey.Command | hotkey.Shift}
	for _, input := range []string{
		"control+command++",
		"control+command+\\+",
		"control+command+plus",
	} {
		if got := mustParse(t, input); got != want {
			t.Errorf("%s: got %+v; want %+v", input, got, want)
		}
	}
}

func TestNamedKeys(t *testing.T) {
	tests := []struct {
		input string
		key   hotkey.Key
	}{
		{"control+command+space", hotkey.KeySpace},
		{"control+command+return", hotkey.KeyReturn},
		{"control+command+tab", hotkey.KeyTab},
		{"control+command+delete", hotkey.KeyDelete},
		{"control+command+escape", hotkey.KeyEscape},
		{"control+command+f1", hotkey.KeyF1},
		{"control+command+f15", hotkey.KeyF15},
	}
	for _, tc := range tests {
		got := mustParse(t, tc.input)
		if got.Key != tc.key || got.Mods != hotkey.Control|hotkey.Command {
			t.Errorf("%s: got %+v; want key %v", tc.input, got, tc.key)
		}
	}
}

func TestBadBindings(t *testing.T) {
	inputs := []string{
		"",
		";",
		"a",
		"left_arrow",
		"control+",
		"control+command+",
		"control+command+nosuchkey",
		"control+command+ctrl",
		"control+command+cmd",
		"control+command+opt",
		"control+command+alt",
		"ctrl+command+a",
		"control+cmd+a",
		"control+opt+a",
		"control+alt+a",
		"meta+control+a",
		"ctl+command+a",
		"control+command+left",
		"control+command+right",
		"control+command+up",
		"control+command+down",
		"control+command+leftarrow",
		"control+command+rightarrow",
		"control+command+uparrow",
		"control+command+downarrow",
		"control+command+enter",
		"control+command+backspace",
		"control+command+esc",
		"control+command+semicolon",
		"control+command+apostrophe",
		"control+command+comma",
		"control+command+leftbracket",
		"control+command+⌘a",
		"control+command+control+a",
		"control+control+a",
		"control+command+command+a",
		"shift+shift+a",
		"option+option+a",
	}
	for _, input := range inputs {
		if got, err := Parse(input); err == nil {
			t.Errorf("Parse(%q) = %+v, want an error", input, got)
		}
	}
}

func TestShiftFromSymbolMergesWithExplicitShift(t *testing.T) {
	implicit := mustParse(t, "control+command+!")
	explicit := mustParse(t, "control+command+shift+!")
	want := hotkey.Combo{Key: hotkey.KeyN1, Mods: hotkey.Control | hotkey.Command | hotkey.Shift}
	if implicit != want || explicit != want {
		t.Errorf("got implicit %+v, explicit %+v; want %+v", implicit, explicit, want)
	}
}
