// Package config loads and saves the user's confnav settings, including the
// tree of categories and files under [category_structure].
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/fath-kip/confnav/internal/kind"
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
	// ScanAs is how a scan runs: ScanUser, ScanRoot or "" (not asked yet).
	ScanAs string `toml:"scan_as"`
	// Keys maps an action name to the keys that trigger it.
	Keys      map[string]KeyList `toml:"keys"`
	Structure Structure          `toml:"category_structure"`
}

// Values of Config.ScanAs.
const (
	ScanUser = "user"
	ScanRoot = "root"
)

// Info tells what Load found.
type Info struct {
	Exists bool // the file was there
	// HasStructure is false when the file has no [category_structure]
	// yet: a first run, or a config written by an older version.
	HasStructure bool
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

// Binding is an action with its default keys and what it does.
type Binding struct {
	Action string
	Keys   KeyList
	Desc   string
}

// DefaultKeys lists every action in help order with its default keys.
// Key names are what the terminal reports: "a", "A" (shift+a), "ctrl+r",
// "enter", "space", "up", "pgdown", "f5" and so on.
var DefaultKeys = []Binding{
	{"up", KeyList{"up", "k"}, "move up"},
	{"down", KeyList{"down", "j"}, "move down"},
	{"collapse", KeyList{"left", "h"}, "collapse, or go to the parent folder"},
	{"expand", KeyList{"right", "l"}, "expand"},
	{"top", KeyList{"home", "g"}, "go to the top"},
	{"bottom", KeyList{"end", "G"}, "go to the bottom"},
	{"page_up", KeyList{"pgup", "ctrl+b"}, "page up"},
	{"page_down", KeyList{"pgdown", "ctrl+f"}, "page down"},
	{"open", KeyList{"enter", "space"}, "open the file, or toggle the folder"},
	{"open_with", KeyList{"o"}, "open once with another editor"},
	{"editor", KeyList{"E"}, "choose the default editor"},
	{"settings", KeyList{"S"}, "open confnav's settings (config.toml)"},
	{"preferences", KeyList{"P"}, "preferences: editor, categories, missing files, scan"},
	{"scan", KeyList{"F"}, "scan the system for more config files"},
	{"add", KeyList{"a"}, "add a file, directory, glob or folder"},
	{"edit", KeyList{"r"}, "rename or edit"},
	{"copy", KeyList{"c"}, "copy to another folder"},
	{"move", KeyList{"m"}, "move to another folder"},
	{"remove", KeyList{"d", "delete"}, "remove from the list (files on disk are never touched)"},
	{"toggle_missing", KeyList{"M"}, "show or hide files that don't exist on this system"},
	{"yank", KeyList{"y"}, "copy the path to the clipboard"},
	{"refresh", KeyList{"ctrl+r", "f5"}, "refresh and reload settings"},
	{"search", KeyList{"/"}, "search everything"},
	{"help", KeyList{"?"}, "help"},
	{"quit", KeyList{"q"}, "quit"},
}

// removedActions were bound in older versions; they are dropped silently.
var removedActions = []string{"restore"}

// KeyDesc returns what an action does.
func KeyDesc(action string) string {
	for _, b := range DefaultKeys {
		if b.Action == action {
			return b.Desc
		}
	}
	return ""
}

// Default returns the settings used when no config file exists. Its
// structure is empty; the first-run preferences fill it in.
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
	for _, a := range removedActions {
		delete(cfg.Keys, a)
	}
	for _, b := range DefaultKeys {
		if _, ok := cfg.Keys[b.Action]; !ok {
			cfg.Keys[b.Action] = append(KeyList(nil), b.Keys...)
		}
	}
}

// Path returns the location of config.toml.
func Path() string { return filepath.Join(xdg.ConfigDir(), "config.toml") }

// Load reads the config at path. A missing file is not an error.
func Load(path string) (Config, Info, error) {
	cfg := Default()
	cfg.Keys = nil // decode the user's keys alone, then add the defaults
	md, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Default(), Info{}, nil
		}
		return Default(), Info{Exists: true}, err
	}
	info := Info{Exists: true, HasStructure: md.IsDefined("category_structure")}
	if cfg.RecentLimit <= 0 {
		cfg.RecentLimit = Default().RecentLimit
	}
	if cfg.DateFormat == "" {
		cfg.DateFormat = Default().DateFormat
	}
	if cfg.ScanAs != ScanUser && cfg.ScanAs != ScanRoot {
		cfg.ScanAs = ""
	}
	fillKeys(&cfg)
	if err := normalize(&cfg.Structure); err != nil {
		return Default(), info, fmt.Errorf("category_structure: %w", err)
	}
	return cfg, info, nil
}

// normalize fills defaults in a hand-edited structure and rejects what
// confnav could not show.
func normalize(s *Structure) error {
	entry := func(e *Entry, where string) error {
		if e.Path == "" {
			return fmt.Errorf("%sentry %q has no path", where, e.Name)
		}
		if e.Kind == "" {
			e.Kind = kind.File
		}
		if e.Kind == kind.Folder || !e.Kind.Valid() {
			return fmt.Errorf("%sentry %q: kind must be file, dir or glob, not %q", where, e.Name, e.Kind)
		}
		if e.Name == "" {
			e.Name = filepath.Base(e.Path)
		}
		return nil
	}
	var folder func(f *Folder, where string) error
	folder = func(f *Folder, where string) error {
		if f.Name == "" {
			return fmt.Errorf("%sa category or folder has no name", where)
		}
		where += f.Name + ": "
		for _, e := range f.Entries {
			if err := entry(e, where); err != nil {
				return err
			}
		}
		for _, c := range f.Folders {
			if err := folder(c, where); err != nil {
				return err
			}
		}
		return nil
	}
	for _, e := range s.Entries {
		if err := entry(e, ""); err != nil {
			return err
		}
	}
	for _, f := range s.Categories {
		if err := folder(f, ""); err != nil {
			return err
		}
	}
	return nil
}

// Save writes cfg to path with comments explaining every setting.
func Save(path string, cfg Config) error {
	return xdg.WriteFileAtomic(path, encode(cfg), 0o644)
}
