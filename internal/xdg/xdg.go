// Package xdg resolves the XDG base directories used by confnav and
// provides an atomic file writer for its data files.
package xdg

import (
	"os"
	"path/filepath"
)

// App is the directory name used under each XDG base directory.
const App = "confnav"

// ConfigHome returns $XDG_CONFIG_HOME or ~/.config.
func ConfigHome() string { return base("XDG_CONFIG_HOME", ".config") }

// DataHome returns $XDG_DATA_HOME or ~/.local/share.
func DataHome() string { return base("XDG_DATA_HOME", ".local/share") }

// StateHome returns $XDG_STATE_HOME or ~/.local/state.
func StateHome() string { return base("XDG_STATE_HOME", ".local/state") }

// ConfigDir returns the confnav configuration directory.
func ConfigDir() string { return filepath.Join(ConfigHome(), App) }

// DataDir returns the confnav data directory.
func DataDir() string { return filepath.Join(DataHome(), App) }

// StateDir returns the confnav state directory.
func StateDir() string { return filepath.Join(StateHome(), App) }

func base(env, fallback string) string {
	// The spec says relative values must be ignored.
	if v := os.Getenv(env); filepath.IsAbs(v) {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/"
	}
	return filepath.Join(home, fallback)
}

// WriteFileAtomic writes data to a temporary file next to path and renames
// it into place, so readers never see a partially written file.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	fail := func(err error) error {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if _, err := f.Write(data); err != nil {
		return fail(err)
	}
	if err := f.Sync(); err != nil {
		return fail(err)
	}
	if err := f.Chmod(perm); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
