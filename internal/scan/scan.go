// Package scan looks through the usual configuration locations for files
// confnav does not list yet and sorts them into categories.
//
// Only known places are searched (/etc, the boot loader, ~/.config and
// well-known dotfiles), never vendor directories such as /usr/lib: files
// there are replaced by package updates, and confnav already offers them as
// defaults to start from.
package scan

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Found is a configuration file the scan came across.
type Found struct {
	Path     string `json:"path"`
	Category string `json:"category"`         // catalog category ID, Apps or Other
	Folder   string `json:"folder,omitempty"` // folder inside the category: the app or /etc directory
	// Group is the directory whose files are listed as the directory itself
	// when there are many of them; empty for the top of a scanned location.
	Group string `json:"group,omitempty"`
}

// Categories the scan creates when it finds files outside the catalog's topics.
const (
	Apps  = "apps"  // ~/.config/<app> and other dotfiles
	Other = "other" // anything else in /etc
)

// Names are the category names for Apps and Other.
var Names = map[string]string{Apps: "Applications", Other: "Other"}

// Roots are the locations a scan searches. Tests point them at temp dirs.
type Roots struct {
	Etc, Boot, Home string
}

// System returns the real locations for the user whose home is home.
func System(home string) Roots { return Roots{Etc: "/etc", Boot: "/boot", Home: home} }

// maxSize is the largest file considered a configuration file.
const maxSize = 1 << 20

// Run scans r and returns what it found, sorted by path. Unreadable
// directories are skipped: run it as root to see them too.
func Run(r Roots) []Found {
	s := &scanner{r: r, seen: map[string]bool{}}
	s.etc()
	s.boot()
	s.home()
	sort.Slice(s.out, func(i, j int) bool { return s.out[i].Path < s.out[j].Path })
	return s.out
}

type scanner struct {
	r    Roots
	out  []Found
	seen map[string]bool
}

func (s *scanner) add(base, root, path string) {
	if s.seen[path] {
		return
	}
	s.seen[path] = true
	rel, _ := filepath.Rel(root, path)
	f := classify(base, rel)
	f.Path = path
	if dir := filepath.Dir(path); dir != root && dir != filepath.Join(s.r.Home, ".config") {
		f.Group = dir
	}
	s.out = append(s.out, f)
}

// skipDirs are never entered anywhere: certificates, keys and keyrings.
var skipDirs = map[string]bool{"ssl": true, "certs": true, "pki": true, "ca-certificates": true, "gnupg": true, ".gnupg": true}

// skipEtc are directories of /etc that hold no settings to edit.
var skipEtc = map[string]bool{"skel": true, "fonts": true, "letsencrypt": true}

func (s *scanner) etc() {
	root := s.r.Etc
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return skip(d)
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(root, path)
			if path != root && (skipDirs[d.Name()] || skipEtc[rel]) {
				return filepath.SkipDir
			}
			return nil
		}
		if isFile(path, d) && wanted(path, d.Name()) {
			s.add("etc", root, path)
		}
		return nil
	})
}

// bootFiles are the boot loader files worth listing.
var bootFiles = []string{
	"loader/loader.conf", "loader/entries/*.conf", "grub/grub.cfg", "grub/custom.cfg",
	"limine.conf", "limine/limine.conf", "EFI/limine/limine.conf",
	"refind_linux.conf", "EFI/refind/refind.conf",
}

func (s *scanner) boot() {
	for _, pat := range bootFiles {
		matches, _ := filepath.Glob(filepath.Join(s.r.Boot, pat))
		for _, m := range matches {
			if fi, err := os.Stat(m); err == nil && fi.Mode().IsRegular() && wanted(m, filepath.Base(m)) {
				s.add("boot", s.r.Boot, m)
			}
		}
	}
}

// dotfiles are configuration files directly in the home directory.
var dotfiles = []string{
	".bashrc", ".bash_profile", ".bash_logout", ".profile", ".zshrc", ".zprofile", ".zshenv", ".zlogin",
	".inputrc", ".gitconfig", ".vimrc", ".nanorc", ".tmux.conf", ".Xresources", ".Xdefaults",
	".xinitrc", ".xprofile", ".xsession", ".asoundrc", ".npmrc", ".wgetrc", ".curlrc",
	".editorconfig", ".ssh/config",
}

// skipConfig are directories in ~/.config that hold browser profiles,
// caches and other state rather than settings.
var skipConfig = map[string]bool{
	"chromium": true, "google-chrome": true, "google-chrome-beta": true, "BraveSoftware": true,
	"vivaldi": true, "microsoft-edge": true, "discord": true, "Slack": true, "Signal": true,
	"Electron": true, "confnav": true, // its own settings are one key away (shift+s)
	"Local Storage": true, "Session Storage": true, "IndexedDB": true, "Crashpad": true,
	"logs": true, "blob_storage": true, "Service Worker": true, "databases": true,
	"WebStorage": true, "workspaceStorage": true, "globalStorage": true, "History": true,
	"Partitions": true, "Dictionaries": true, "sessions": true, "history": true, "backups": true,
}

// configDepth is how many directories deep ~/.config is searched below each app.
const configDepth = 3

func (s *scanner) home() {
	home := s.r.Home
	for _, name := range dotfiles {
		p := filepath.Join(home, name)
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() && wanted(p, filepath.Base(p)) {
			s.add("home", home, p)
		}
	}

	config := filepath.Join(home, ".config")
	_ = filepath.WalkDir(config, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return skip(d)
		}
		rel, _ := filepath.Rel(config, path)
		if d.IsDir() {
			if path != config && (skipDirs[d.Name()] || skipConfig[d.Name()] ||
				strings.Contains(strings.ToLower(d.Name()), "cache") || strings.Count(rel, "/") >= configDepth) {
				return filepath.SkipDir
			}
			return nil
		}
		if isFile(path, d) && configLike(d.Name()) && wanted(path, d.Name()) {
			s.add("home", home, path)
		}
		return nil
	})

	apps, _ := filepath.Glob(filepath.Join(home, ".local/share/applications/*.desktop"))
	for _, p := range apps {
		if wanted(p, filepath.Base(p)) {
			s.add("home", home, p)
		}
	}
}

// skip is what WalkDir should do after an error on d.
func skip(d fs.DirEntry) error {
	if d != nil && d.IsDir() {
		return filepath.SkipDir
	}
	return nil
}

// isFile reports whether d is a regular file or a symlink to one; links to
// directories are not followed.
func isFile(path string, d fs.DirEntry) bool {
	if d.Type().IsRegular() {
		return true
	}
	if d.Type()&fs.ModeSymlink != 0 {
		fi, err := os.Stat(path)
		return err == nil && fi.Mode().IsRegular()
	}
	return false
}

// skipSuffixes mark backups, package manager leftovers, keys, archives and
// other files that are not settings to edit.
var skipSuffixes = []string{
	".pacnew", ".pacsave", ".pacorig", ".rpmnew", ".rpmsave", ".rpmorig", ".dpkg-old", ".dpkg-new",
	".dpkg-dist", ".ucf-dist", ".bak", ".old", ".orig", ".swp", ".lock", ".pid", "~", "-",
	".key", ".pem", ".crt", ".cer", ".der", ".p12", ".pfx", ".gpg", ".kbx", ".keyring",
	".db", ".sqlite", ".cache", ".gz", ".xz", ".zst", ".bz2", ".zip", ".png", ".jpg", ".svg",
	".ico", ".mo", ".so", ".bin", ".hwdb",
}

// wanted reports whether a file looks like readable settings: not a
// backup or key, not huge and not binary. Files we may not read are kept,
// since in /etc they usually are settings (sudoers, shadow).
func wanted(path, name string) bool {
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "ssh_host_") {
		return false
	}
	for _, s := range skipSuffixes {
		if strings.HasSuffix(lower, s) {
			return false
		}
	}
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	if fi.Size() > maxSize {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return os.IsPermission(err)
	}
	defer f.Close()
	buf := make([]byte, 8192)
	n, _ := io.ReadFull(f, buf)
	return !bytes.Contains(buf[:n], []byte{0})
}

// configExts are the file types found in ~/.config that hold settings.
var configExts = map[string]bool{
	".conf": true, ".cfg": true, ".ini": true, ".toml": true, ".yaml": true, ".yml": true,
	".json": true, ".jsonc": true, ".json5": true, ".lua": true, ".vim": true, ".kdl": true,
	".rasi": true, ".css": true, ".ron": true, ".rc": true, ".list": true, ".dirs": true,
	".properties": true, ".fish": true, ".zsh": true, ".desktop": true, ".theme": true,
}

// configLike reports whether a file name in ~/.config looks like settings
// rather than state.
func configLike(name string) bool {
	lower := strings.ToLower(name)
	return configExts[filepath.Ext(lower)] || lower == "config" || lower == "settings" ||
		strings.HasSuffix(lower, "rc") || strings.Contains(lower, "conf") || lower == "kdeglobals"
}
