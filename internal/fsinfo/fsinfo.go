// Package fsinfo answers filesystem questions for the UI: does a path exist,
// may we write it, what is inside a directory or glob.
package fsinfo

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/fath-kip/confnav/internal/xdg"
)

// State is the result of looking a path up.
type State int

const (
	Missing State = iota
	Present
	NoAccess // a parent directory is not readable by us
)

// Info is what the UI shows about a path.
type Info struct {
	State   State
	IsDir   bool
	ModTime time.Time
}

// Expand resolves a leading ~ and $VARS. XDG variables fall back to their
// defaults when unset, so "$XDG_CONFIG_HOME/foo" always means something.
func Expand(p string) string {
	home, _ := os.UserHomeDir()
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		p = filepath.Join(home, p[2:])
	}
	if !strings.Contains(p, "$") {
		return p
	}
	return os.Expand(p, func(k string) string {
		switch k {
		case "HOME":
			return home
		case "XDG_CONFIG_HOME":
			return xdg.ConfigHome()
		case "XDG_DATA_HOME":
			return xdg.DataHome()
		case "XDG_STATE_HOME":
			return xdg.StateHome()
		}
		return os.Getenv(k)
	})
}

// Abbrev shortens paths under the home directory to ~/...
func Abbrev(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || home == "/" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+"/") {
		return "~" + p[len(home):]
	}
	return p
}

// Stat looks up an already expanded path.
func Stat(p string) Info {
	fi, err := os.Stat(p)
	switch {
	case err == nil:
		return Info{State: Present, IsDir: fi.IsDir(), ModTime: fi.ModTime()}
	case errors.Is(err, fs.ErrPermission):
		return Info{State: NoAccess}
	default:
		return Info{State: Missing}
	}
}

const wOK = 2 // W_OK for access(2)

// Writable reports whether the current user can save p: either the file is
// writable, or it does not exist yet and its directory is writable.
func Writable(p string) bool {
	if syscall.Access(p, wOK) == nil {
		return true
	}
	if _, err := os.Lstat(p); errors.Is(err, fs.ErrNotExist) {
		return syscall.Access(filepath.Dir(p), wOK) == nil
	}
	return false
}

// Child is one entry of a directory listing or glob expansion.
type Child struct {
	Path  string
	IsDir bool
}

// ListDir lists dir with directories first, then files, each sorted by name.
func ListDir(dir string) ([]Child, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]Child, 0, len(entries))
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		isDir := e.IsDir()
		if e.Type()&fs.ModeSymlink != 0 {
			if fi, err := os.Stat(p); err == nil {
				isDir = fi.IsDir()
			}
		}
		out = append(out, Child{Path: p, IsDir: isDir})
	}
	sortChildren(out)
	return out, nil
}

// Glob expands pattern. A pattern that matches nothing is not an error.
func Glob(pattern string) ([]Child, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	out := make([]Child, 0, len(matches))
	for _, m := range matches {
		out = append(out, Child{Path: m, IsDir: Stat(m).IsDir})
	}
	sortChildren(out)
	return out, nil
}

func sortChildren(c []Child) {
	sort.SliceStable(c, func(i, j int) bool {
		if c[i].IsDir != c[j].IsDir {
			return c[i].IsDir
		}
		return c[i].Path < c[j].Path
	})
}

// HasGlob reports whether p contains glob metacharacters.
func HasGlob(p string) bool { return strings.ContainsAny(p, "*?[") }

// Complete does shell-like completion of a path typed by the user. It returns
// the input extended by the longest common prefix of the matches, and the
// matches themselves when there is more than one. Directories end in "/".
func Complete(input string) (string, []string) {
	if input == "" {
		return "/", nil
	}
	if input == "~" {
		return "~/", nil
	}
	exp := Expand(input)
	if strings.HasSuffix(input, "/") && !strings.HasSuffix(exp, "/") {
		exp += "/"
	}
	dir, base := filepath.Split(exp)
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return input, nil
	}
	var matches []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, base) || (strings.HasPrefix(name, ".") && !strings.HasPrefix(base, ".")) {
			continue
		}
		if e.IsDir() || (e.Type()&fs.ModeSymlink != 0 && Stat(filepath.Join(dir, name)).IsDir) {
			name += "/"
		}
		matches = append(matches, name)
	}
	if len(matches) == 0 {
		return input, nil
	}
	sort.Strings(matches)
	common := matches[0]
	for _, m := range matches[1:] {
		common = commonPrefix(common, m)
	}
	completed := input[:len(input)-len(base)] + common
	if len(matches) == 1 {
		return completed, nil
	}
	return completed, matches
}

func commonPrefix(a, b string) string {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return a[:i]
		}
	}
	return a[:n]
}

// Backup copies src into dir under a timestamped name and returns the copy's path.
func Backup(src, dir string, now time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := now.Format("20060102-150405") + "_" + strings.ReplaceAll(strings.TrimPrefix(src, "/"), "/", "_")
	dst := filepath.Join(dir, name)
	return dst, CopyFile(src, dst, 0o600)
}

// CopyFile copies src to a new file dst; it never overwrites.
func CopyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}
