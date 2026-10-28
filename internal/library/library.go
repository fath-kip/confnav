// Package library reads library.json, where versions before
// [category_structure] kept what the user changed on top of the built-in
// catalog: their own folders and entries, hidden built-ins and renames. It
// is only used to move that into config.toml once.
package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/fath-kip/confnav/internal/catalog"
	"github.com/fath-kip/confnav/internal/config"
	"github.com/fath-kip/confnav/internal/distro"
	"github.com/fath-kip/confnav/internal/fsinfo"
	"github.com/fath-kip/confnav/internal/kind"
	"github.com/fath-kip/confnav/internal/xdg"
)

// Node is a user folder or entry.
type Node struct {
	ID       string    `json:"id"`
	Kind     kind.Kind `json:"kind"`
	Name     string    `json:"name"`
	Desc     string    `json:"desc,omitempty"`
	Path     string    `json:"path,omitempty"`
	Children []*Node   `json:"children,omitempty"`
}

// Override replaces the name and description of a built-in item.
type Override struct {
	Name string `json:"name,omitempty"`
	Desc string `json:"desc,omitempty"`
}

// Extra holds the user nodes added to one built-in category.
type Extra struct {
	Category string  `json:"category"` // catalog.CategoryID
	Nodes    []*Node `json:"nodes"`
}

// Library is the content of library.json.
type Library struct {
	Version int `json:"version"`
	// Folders are the user's top-level nodes: folders, and files or
	// directories added at the top level.
	Folders []*Node `json:"folders"`
	// Extras are user nodes added inside built-in categories.
	Extras []Extra `json:"extras,omitempty"`
	// Hidden lists built-in category and entry IDs the user removed.
	Hidden []string `json:"hidden,omitempty"`
	// Overrides holds user renames of built-in categories and entries.
	Overrides map[string]Override `json:"overrides,omitempty"`
}

// Path returns the location of library.json.
func Path() string { return filepath.Join(xdg.DataDir(), "library.json") }

// MigratedPath is where library.json is kept after moving into config.toml.
func MigratedPath() string { return Path() + ".migrated" }

// Load reads library.json. A missing file returns nil without an error.
func Load(path string) (*Library, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	l := &Library{}
	if err := json.Unmarshal(data, l); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return l, nil
}

// Migrate turns the library and the catalog it was layered on into a
// structure for config.toml that shows the same tree: the user's top-level
// folders first, then the catalog categories that were not hidden, with
// renames applied and the user's extras inside them.
func Migrate(l *Library, c *catalog.Catalog, f distro.Family) config.Structure {
	known := map[string]*config.Entry{} // expanded path → catalog entry, for hints
	var cats []*config.Folder
	for _, cat := range c.Categories {
		cid := catalog.CategoryID(cat.ID)
		folder := &config.Folder{Catalog: cat.ID, Name: cat.Name}
		if o, ok := l.Overrides[cid]; ok && o.Name != "" {
			folder.Name = o.Name
		}
		for _, e := range cat.Entries {
			if !e.For(f) {
				continue
			}
			ce := &config.Entry{Name: e.Name, Desc: e.Desc, Path: e.Path, Kind: e.Kind,
				Hint: e.HintFor(f), Defaults: e.Defaults}
			if _, ok := known[fsinfo.Expand(e.Path)]; !ok {
				known[fsinfo.Expand(e.Path)] = ce.Clone() // before renames, as the tree showed it
			}
			id := catalog.EntryID(cat.ID, e.Path)
			if slices.Contains(l.Hidden, id) {
				continue
			}
			if o, ok := l.Overrides[id]; ok {
				if o.Name != "" {
					ce.Name = o.Name
				}
				if o.Desc != "" {
					ce.Desc = o.Desc
				}
			}
			folder.Entries = append(folder.Entries, ce)
		}
		if !slices.Contains(l.Hidden, cid) {
			cats = append(cats, folder)
		}
	}

	var s config.Structure
	// convert adds n to into (nil: the top level).
	var convert func(n *Node, into *config.Folder)
	convert = func(n *Node, into *config.Folder) {
		if n.Kind == kind.Folder {
			sub := &config.Folder{Name: n.Name}
			for _, ch := range n.Children {
				convert(ch, sub)
			}
			s.AddFolder(into, sub)
			return
		}
		e := &config.Entry{Name: n.Name, Desc: n.Desc, Path: n.Path, Kind: n.Kind}
		if e.Kind == "" {
			e.Kind = kind.File
		}
		if k, ok := known[fsinfo.Expand(n.Path)]; ok {
			e.Hint, e.Defaults = k.Hint, k.Defaults
			if e.Desc == "" {
				e.Desc = k.Desc
			}
		}
		s.AddEntry(into, e)
	}
	for _, n := range l.Folders {
		convert(n, nil)
	}
	for _, folder := range cats {
		for _, ex := range l.Extras {
			if ex.Category == catalog.CategoryID(folder.Catalog) {
				for _, n := range ex.Nodes {
					convert(n, folder)
				}
			}
		}
		if len(folder.Entries) > 0 || len(folder.Folders) > 0 {
			s.Categories = append(s.Categories, folder)
		}
	}
	return s
}
