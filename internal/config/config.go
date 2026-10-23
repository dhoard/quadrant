// Package config handles the persistent, user-editable configuration: it
// creates the file on first run, validates its grid and shortcut regions on
// every load, and — through Watch — re-loads it while the program runs.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"quadrant/internal/geometry"
)

// DefaultDirectory is the directory under the user's home that holds the
// configuration file, so the default path is ~/.quadrant/configuration.json.
const DefaultDirectory = ".quadrant"

// FileName is the configuration file's name inside DefaultDirectory.
const FileName = "configuration.json"

// formatHint makes a decode failure actionable when an older named-placement
// file is encountered. It is diagnostics only: no legacy decoding is done.
const formatHint = " (the configuration format uses top-level rows and columns with a shortcuts object keyed by key combination; see README.md)"

// ShortcutMap maps a key-combination string to a grid region. The custom
// decoder rejects duplicate JSON object keys, which encoding/json would
// otherwise silently collapse to the last value.
type ShortcutMap map[string]geometry.Region

type Configuration struct {
	Rows          int         `json:"rows"`
	Columns       int         `json:"columns"`
	Shortcuts     ShortcutMap `json:"shortcuts"`
	PaddingPoints float64     `json:"padding_points"`
}

func Default() Configuration {
	return Configuration{
		Rows:    2,
		Columns: 2,
		Shortcuts: ShortcutMap{
			"control+command+;":           {Row: 1, Column: 1, RowSpan: 1, ColumnSpan: 1},
			"control+command+'":           {Row: 1, Column: 2, RowSpan: 1, ColumnSpan: 1},
			"control+command+.":           {Row: 2, Column: 1, RowSpan: 1, ColumnSpan: 1},
			"control+command+/":           {Row: 2, Column: 2, RowSpan: 1, ColumnSpan: 1},
			"control+command+left_arrow":  {Row: 1, Column: 1, RowSpan: 2, ColumnSpan: 1},
			"control+command+right_arrow": {Row: 1, Column: 2, RowSpan: 2, ColumnSpan: 1},
			"control+command+up_arrow":    {Row: 1, Column: 1, RowSpan: 1, ColumnSpan: 2},
			"control+command+down_arrow":  {Row: 2, Column: 1, RowSpan: 1, ColumnSpan: 2},
		},
		PaddingPoints: 0,
	}
}

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return filepath.Join(home, DefaultDirectory, FileName), nil
}

// regionJSON distinguishes an omitted span from an explicit zero so optional
// spans can default to one while zero remains a validation error.
type regionJSON struct {
	Row        *int `json:"row"`
	Column     *int `json:"column"`
	RowSpan    *int `json:"row_span"`
	ColumnSpan *int `json:"column_span"`
}

func decodeRegion(data []byte) (geometry.Region, error) {
	var raw regionJSON
	if err := decodeStrict(data, &raw); err != nil {
		return geometry.Region{}, err
	}
	if raw.Row == nil {
		return geometry.Region{}, errors.New("row is required")
	}
	if raw.Column == nil {
		return geometry.Region{}, errors.New("column is required")
	}
	rowSpan, columnSpan := 1, 1
	if raw.RowSpan != nil {
		rowSpan = *raw.RowSpan
	}
	if raw.ColumnSpan != nil {
		columnSpan = *raw.ColumnSpan
	}
	return geometry.Region{
		Row:        *raw.Row,
		Column:     *raw.Column,
		RowSpan:    rowSpan,
		ColumnSpan: columnSpan,
	}, nil
}

// decodeStrict decodes one JSON value with unknown fields rejected and no
// trailing value allowed.
func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

// UnmarshalJSON decodes the shortcuts object while preserving the distinction
// between two textually identical keys, which a plain map would lose.
func (m *ShortcutMap) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return errors.New("shortcuts must be an object")
	}

	out := ShortcutMap{}
	for dec.More() {
		tok, err = dec.Token()
		if err != nil {
			return err
		}
		key, ok := tok.(string)
		if !ok {
			return errors.New("shortcuts keys must be strings")
		}
		if _, exists := out[key]; exists {
			return fmt.Errorf("duplicate shortcut %q", key)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return fmt.Errorf("shortcut %q: %w", key, err)
		}
		region, err := decodeRegion(raw)
		if err != nil {
			return fmt.Errorf("shortcut %q: %w", key, err)
		}
		out[key] = region
	}
	if _, err := dec.Token(); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("shortcuts: multiple JSON values")
		}
		return err
	}
	*m = out
	return nil
}

func (c Configuration) Validate() error {
	if c.PaddingPoints < 0 || c.PaddingPoints > 128 || math.IsNaN(c.PaddingPoints) {
		return fmt.Errorf("padding_points must be between 0 and 128")
	}
	grid := geometry.Grid{Rows: c.Rows, Columns: c.Columns}
	if err := grid.Validate(); err != nil {
		return err
	}
	if len(c.Shortcuts) == 0 {
		return errors.New("shortcuts must contain at least one binding")
	}
	keys := make([]string, 0, len(c.Shortcuts))
	for key := range c.Shortcuts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			return errors.New("shortcuts contains an empty binding")
		}
		if err := grid.ValidateRegion(c.Shortcuts[key]); err != nil {
			return fmt.Errorf("shortcuts[%q]: %w", key, err)
		}
	}
	return nil
}

// LoadOrCreate saves the default config on first launch. Existing configuration
// is never overwritten, even if it is invalid.
//
// It also returns the raw file content the configuration was decoded from, so
// that Watch can compare later polls against exactly what is in service rather
// than re-reading the file and getting it a second time, which would swallow a
// change made in between.
func LoadOrCreate(path string) (Configuration, []byte, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		def := Default()
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return Configuration{}, nil, fmt.Errorf("create config directory: %w", err)
		}
		b, err = json.MarshalIndent(def, "", "  ")
		if err != nil {
			return Configuration{}, nil, err
		}
		b = append(b, '\n')
		// Exclusive creation prevents replacing a file created by another run.
		f, createErr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(createErr, os.ErrExist) {
			return LoadOrCreate(path)
		}
		if createErr != nil {
			return Configuration{}, nil, fmt.Errorf("create config: %w", createErr)
		}
		_, writeErr := f.Write(b)
		closeErr := f.Close()
		if writeErr != nil {
			return Configuration{}, nil, fmt.Errorf("write config: %w", writeErr)
		}
		if closeErr != nil {
			return Configuration{}, nil, fmt.Errorf("close config: %w", closeErr)
		}
	} else if err != nil {
		return Configuration{}, nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Configuration
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Configuration{}, nil, fmt.Errorf("parse %s: %w%s", path, err, formatHint)
	}
	// Reject concatenated JSON documents as well as trailing garbage.
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Configuration{}, nil, fmt.Errorf("parse %s: multiple JSON values%s", path, formatHint)
		}
		return Configuration{}, nil, fmt.Errorf("parse %s: %w%s", path, err, formatHint)
	}
	if err := cfg.Validate(); err != nil {
		return Configuration{}, nil, fmt.Errorf("invalid %s: %w", path, err)
	}
	return cfg, b, nil
}
