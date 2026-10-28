package config

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/fath-kip/confnav/internal/fsinfo"
	"github.com/fath-kip/confnav/internal/kind"
)

// Structure is the tree confnav shows: categories with their folders and
// entries. It lives under [category_structure] in config.toml so it can be
// edited by hand as well as in the app.
type Structure struct {
	// Ignore lists paths the user removed after a scan, so a new scan
	// does not bring them back.
	Ignore     []string  `toml:"ignore"`
	Categories []*Folder `toml:"category"`
	// Entries are files, directories and globs added at the top level.
	Entries []*Entry `toml:"entry"`
}

// Folder is a category (at the top level) or a folder inside one.
type Folder struct {
	// Catalog ties a category to the built-in catalog ("dns", "network", …),
	// so preferences and scans know which topic it is. Empty for your own.
	Catalog string    `toml:"catalog"`
	Name    string    `toml:"name"`
	Scanned bool      `toml:"found_by_scan"`
	Entries []*Entry  `toml:"entry"`
	Folders []*Folder `toml:"folder"`
}

// Entry is a file, directory or glob.
type Entry struct {
	Name     string    `toml:"name"`
	Desc     string    `toml:"desc"`
	Path     string    `toml:"path"`
	Kind     kind.Kind `toml:"kind"`
	Hint     string    `toml:"hint"`
	Defaults string    `toml:"defaults"`
	Scanned  bool      `toml:"found_by_scan"`
}

// Category returns the top-level folder tied to a catalog ID.
func (s *Structure) Category(catalogID string) *Folder {
	for _, f := range s.Categories {
		if f.Catalog == catalogID {
			return f
		}
	}
	return nil
}

func (s *Structure) folders(parent *Folder) *[]*Folder {
	if parent == nil {
		return &s.Categories
	}
	return &parent.Folders
}

func (s *Structure) entries(parent *Folder) *[]*Entry {
	if parent == nil {
		return &s.Entries
	}
	return &parent.Entries
}

// AddFolder appends f to parent (nil: the top level, where it is a category).
func (s *Structure) AddFolder(parent, f *Folder) {
	l := s.folders(parent)
	*l = append(*l, f)
}

// AddEntry appends e to parent (nil: the top level).
func (s *Structure) AddEntry(parent *Folder, e *Entry) {
	l := s.entries(parent)
	*l = append(*l, e)
}

// RemoveFolder takes f out of parent.
func (s *Structure) RemoveFolder(parent, f *Folder) bool {
	l := s.folders(parent)
	i := slices.Index(*l, f)
	if i < 0 {
		return false
	}
	*l = slices.Delete(*l, i, i+1)
	return true
}

// RemoveEntry takes e out of parent.
func (s *Structure) RemoveEntry(parent *Folder, e *Entry) bool {
	l := s.entries(parent)
	i := slices.Index(*l, e)
	if i < 0 {
		return false
	}
	*l = slices.Delete(*l, i, i+1)
	return true
}

// Holds reports whether g is f or somewhere below it.
func (f *Folder) Holds(g *Folder) bool {
	if f == g {
		return true
	}
	for _, c := range f.Folders {
		if c.Holds(g) {
			return true
		}
	}
	return false
}

// Count returns the number of entries in f and its folders.
func (f *Folder) Count() int {
	n := len(f.Entries)
	for _, c := range f.Folders {
		n += c.Count()
	}
	return n
}

// Clone deep-copies f.
func (f *Folder) Clone() *Folder {
	c := *f
	c.Entries, c.Folders = nil, nil
	for _, e := range f.Entries {
		c.Entries = append(c.Entries, e.Clone())
	}
	for _, sub := range f.Folders {
		c.Folders = append(c.Folders, sub.Clone())
	}
	return &c
}

// Clone copies e.
func (e *Entry) Clone() *Entry {
	c := *e
	return &c
}

// Walk calls fn for every entry, top-level ones included.
func (s *Structure) Walk(fn func(e *Entry)) {
	for _, e := range s.Entries {
		fn(e)
	}
	for _, f := range s.Categories {
		f.walk(fn)
	}
}

// walk calls fn for every entry in f and its folders.
func (f *Folder) walk(fn func(e *Entry)) {
	for _, e := range f.Entries {
		fn(e)
	}
	for _, c := range f.Folders {
		c.walk(fn)
	}
}

// ScannedPaths returns the paths of the entries in f that a scan added.
func (f *Folder) ScannedPaths() []string {
	var out []string
	f.walk(func(e *Entry) {
		if e.Scanned {
			out = append(out, e.Path)
		}
	})
	return out
}

// Index answers "is this path already listed?" for many paths at once.
type Index struct {
	files map[string]bool
	dirs  []string
	globs []string
}

// Index collects the expanded paths of every entry.
func (s *Structure) Index() *Index {
	x := &Index{files: map[string]bool{}}
	s.Walk(func(e *Entry) {
		p := fsinfo.Expand(e.Path)
		switch e.Kind {
		case kind.Dir:
			x.dirs = append(x.dirs, p)
		case kind.Glob:
			x.globs = append(x.globs, p)
		default:
			x.files[p] = true
		}
	})
	return x
}

// Covers reports whether path (expanded) is listed: as an entry, inside a
// listed directory or matched by a listed glob.
func (x *Index) Covers(path string) bool {
	if x.files[path] {
		return true
	}
	for _, d := range x.dirs {
		if path == d || strings.HasPrefix(path, d+"/") {
			return true
		}
	}
	for _, g := range x.globs {
		if ok, _ := filepath.Match(g, path); ok {
			return true
		}
	}
	return false
}

// Ignored reports whether path (expanded) is in the ignore list.
func (s *Structure) Ignored(path string) bool {
	for _, p := range s.Ignore {
		if fsinfo.Expand(p) == path {
			return true
		}
	}
	return false
}

// AddIgnore adds paths to the ignore list once each.
func (s *Structure) AddIgnore(paths ...string) {
	for _, p := range paths {
		if !s.Ignored(fsinfo.Expand(p)) {
			s.Ignore = append(s.Ignore, p)
		}
	}
}
