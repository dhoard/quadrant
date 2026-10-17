// Package config handles the persistent, user-editable configuration: it
// creates the file on first run, validates it on every load, and — through
// Watch — re-loads it while the program runs.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// DefaultDirectory is the directory under the user's home that holds the
// configuration file, so the default path is ~/.quadrant/configuration.json.
const DefaultDirectory = ".quadrant"

// FileName is the configuration file's name inside DefaultDirectory.
const FileName = "configuration.json"

type Shortcuts struct {
	TopLeft     string `json:"top_left"`
	TopRight    string `json:"top_right"`
	BottomLeft  string `json:"bottom_left"`
	BottomRight string `json:"bottom_right"`

	// Half-screen placements. They are optional: an empty binding is not
	// registered, so a configuration written before they existed stays valid.
	LeftHalf   string `json:"left_half"`
	RightHalf  string `json:"right_half"`
	TopHalf    string `json:"top_half"`
	BottomHalf string `json:"bottom_half"`
}

type Configuration struct {
	Shortcuts     Shortcuts `json:"shortcuts"`
	PaddingPoints float64   `json:"padding_points"`
}

func Default() Configuration {
	return Configuration{
		Shortcuts: Shortcuts{
			TopLeft: "control+option+;", TopRight: "control+option+'",
			BottomLeft: "control+option+,", BottomRight: "control+option+/",
			LeftHalf: "control+option+left", RightHalf: "control+option+right",
			TopHalf: "control+option+up", BottomHalf: "control+option+down",
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

// Shortcut pairs a placement's configuration name with its binding. Required
// marks the placements every configuration must define.
type Shortcut struct {
	Name     string
	Binding  string
	Required bool
}

func (s Shortcuts) Entries() []Shortcut {
	return []Shortcut{
		{"top_left", s.TopLeft, true},
		{"top_right", s.TopRight, true},
		{"bottom_left", s.BottomLeft, true},
		{"bottom_right", s.BottomRight, true},
		{"left_half", s.LeftHalf, false},
		{"right_half", s.RightHalf, false},
		{"top_half", s.TopHalf, false},
		{"bottom_half", s.BottomHalf, false},
	}
}

func (c Configuration) Validate() error {
	if c.PaddingPoints < 0 || c.PaddingPoints > 128 || c.PaddingPoints != c.PaddingPoints {
		return fmt.Errorf("padding_points must be between 0 and 128")
	}
	for _, entry := range c.Shortcuts.Entries() {
		if strings.TrimSpace(entry.Binding) == "" && entry.Required {
			return fmt.Errorf("shortcuts.%s is required", entry.Name)
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
		return Configuration{}, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	// Reject concatenated JSON documents as well as trailing garbage.
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Configuration{}, nil, fmt.Errorf("parse %s: multiple JSON values", path)
		}
		return Configuration{}, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Configuration{}, nil, fmt.Errorf("invalid %s: %w", path, err)
	}
	return cfg, b, nil
}
