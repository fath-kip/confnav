// Package config loads and saves the user's confnav settings.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/fath-kip/confnav/internal/xdg"
)

// Config holds user settings from config.toml.
type Config struct {
	Editor      string `toml:"editor"`
	Backup      bool   `toml:"backup"`
	Mouse       bool   `toml:"mouse"`
	RecentLimit int    `toml:"recent_limit"`
	DateFormat  string `toml:"date_format"`
	ShowMissing bool   `toml:"show_missing"`
	// Keys maps an action name to the keys that trigger it.
	Keys map[string]KeyList `toml:"keys"`
}

// KeyList is one or more keys; in TOML either "a" or ["a", "f2"].
type KeyList []string

// UnmarshalTOML accepts a single string as well as an array of strings.
func (k *KeyList) UnmarshalTOML(v any) error {
	switch v := v.(type) {
	case string:
		*k = KeyList{v}
		return nil
	case []any:
		out := make(KeyList, 0, len(v))
		for _, e := range v {
			s, ok := e.(string)
			if !ok {
				return fmt.Errorf("keys must be strings, got %v", e)
			}
			out = append(out, s)
		}
		*k = out
		return nil
	}
	return fmt.Errorf("keys must be a string or a list of strings, got %v", v)
}

// Binding is an action with its default keys.
type Binding struct {
	Action string
	Keys   KeyList
}

// DefaultKeys lists every action in help order with its default keys.
// Key names are what the terminal reports: "a", "A" (shift+a), "ctrl+r",
// "enter", "space", "up", "pgdown", "f5" and so on.
var DefaultKeys = []Binding{
	{"up", KeyList{"up", "k"}},
	{"down", KeyList{"down", "j"}},
	{"collapse", KeyList{"left", "h"}},
	{"expand", KeyList{"right", "l"}},
	{"top", KeyList{"home", "g"}},
	{"bottom", KeyList{"end", "G"}},
	{"page_up", KeyList{"pgup", "ctrl+b"}},
	{"page_down", KeyList{"pgdown", "ctrl+f"}},
	{"open", KeyList{"enter", "space"}},
	{"open_with", KeyList{"o"}},
	{"editor", KeyList{"E"}},
	{"add", KeyList{"a"}},
	{"edit", KeyList{"r"}},
	{"copy", KeyList{"c"}},
	{"move", KeyList{"m"}},
	{"remove", KeyList{"d", "delete"}},
	{"restore", KeyList{"R"}},
	{"toggle_missing", KeyList{"M"}},
	{"yank", KeyList{"y"}},
	{"refresh", KeyList{"ctrl+r", "f5"}},
	{"search", KeyList{"/"}},
	{"help", KeyList{"?"}},
	{"quit", KeyList{"q"}},
}

const header = `# confnav configuration
#
# editor        command used to open files, e.g. "micro", "nano", "vim", "nvim -p".
#               Empty means: $VISUAL, then $EDITOR, then the first installed editor.
# backup        copy a file to ~/.local/state/confnav/backups before opening it.
# mouse         enable mouse clicks and scrolling (hold Shift to select text).
# recent_limit  how many files the Recent list keeps.
# date_format   Go time layout for the Modified column
#               ("2006-01-02 15:04" means YYYY-MM-DD HH:MM).
# show_missing  list built-in files that don't exist on this system (toggle: shift+m).
# [keys]        keys for each action: a single key ("a") or a list (["r", "f2"]).
#               Upper case means shift ("E" is shift+e). ctrl+c always quits.
#
# confnav rewrites this file when you change settings in the app (shift+e,
# shift+m), so comments other than this header are not kept.

`

// Default returns the settings used when no config file exists.
func Default() Config {
	cfg := Config{Mouse: true, RecentLimit: 20, DateFormat: "2006-01-02 15:04", ShowMissing: true}
	fillKeys(&cfg)
	return cfg
}

// fillKeys gives every action the user did not configure its default keys.
func fillKeys(cfg *Config) {
	if cfg.Keys == nil {
		cfg.Keys = map[string]KeyList{}
	}
	for _, b := range DefaultKeys {
		if _, ok := cfg.Keys[b.Action]; !ok {
			cfg.Keys[b.Action] = append(KeyList(nil), b.Keys...)
		}
	}
}

// Path returns the location of config.toml.
func Path() string { return filepath.Join(xdg.ConfigDir(), "config.toml") }

// Load reads the config at path. exists reports whether the file was found;
// a missing file is not an error.
func Load(path string) (cfg Config, exists bool, err error) {
	cfg = Default()
	cfg.Keys = nil // decode the user's keys alone, then add the defaults
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Default(), false, nil
		}
		return Default(), true, err
	}
	if cfg.RecentLimit <= 0 {
		cfg.RecentLimit = Default().RecentLimit
	}
	if cfg.DateFormat == "" {
		cfg.DateFormat = Default().DateFormat
	}
	fillKeys(&cfg)
	return cfg, true, nil
}

// Save writes cfg to path with an explanatory header.
func Save(path string, cfg Config) error {
	var buf bytes.Buffer
	buf.WriteString(header)
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return err
	}
	return xdg.WriteFileAtomic(path, buf.Bytes(), 0o644)
}
