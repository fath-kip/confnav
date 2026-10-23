// Package catalog holds the built-in list of well-known configuration files.
package catalog

import (
	_ "embed"
	"fmt"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/fath-kip/confnav/internal/distro"
	"github.com/fath-kip/confnav/internal/kind"
)

//go:embed catalog.toml
var builtin []byte

// Catalog is the parsed catalog.toml.
type Catalog struct {
	Categories []Category `toml:"category"`
}

// Category is a top-level folder of the catalog.
type Category struct {
	ID      string  `toml:"id"`
	Name    string  `toml:"name"`
	Entries []Entry `toml:"entry"`
}

// Entry is one well-known file, directory or glob.
type Entry struct {
	Name     string            `toml:"name"`
	Desc     string            `toml:"desc"`
	Path     string            `toml:"path"`
	Kind     kind.Kind         `toml:"kind"`
	Distros  []distro.Family   `toml:"distros"`
	Hint     string            `toml:"hint"`
	Hints    map[string]string `toml:"hints"`
	Defaults string            `toml:"defaults"`
}

// CategoryID is the stable ID of a category, used for hiding it.
func CategoryID(id string) string { return "cat:" + id }

// EntryID is the stable ID of an entry inside a category.
func EntryID(catID, path string) string { return catID + ":" + path }

// For reports whether the entry applies to family f. Entries without a
// distros list apply everywhere, including unknown distributions.
func (e Entry) For(f distro.Family) bool {
	return len(e.Distros) == 0 || slices.Contains(e.Distros, f)
}

// HintFor returns the after-edit hint for family f.
func (e Entry) HintFor(f distro.Family) string {
	if h, ok := e.Hints[string(f)]; ok {
		return h
	}
	return e.Hint
}

// Load parses the catalog embedded in the binary.
func Load() (*Catalog, error) { return Parse(builtin) }

// Parse decodes and validates catalog data. Unknown keys are an error so
// typos in catalog.toml fail the tests instead of being silently ignored.
func Parse(data []byte) (*Catalog, error) {
	var c Catalog
	md, err := toml.Decode(string(data), &c)
	if err != nil {
		return nil, err
	}
	if undec := md.Undecoded(); len(undec) > 0 {
		keys := make([]string, len(undec))
		for i, k := range undec {
			keys[i] = k.String()
		}
		return nil, fmt.Errorf("catalog: unknown keys: %s", strings.Join(keys, ", "))
	}
	for ci := range c.Categories {
		cat := &c.Categories[ci]
		if cat.ID == "" || cat.Name == "" {
			return nil, fmt.Errorf("catalog: category %d needs id and name", ci+1)
		}
		for ei := range cat.Entries {
			e := &cat.Entries[ei]
			if e.Kind == "" {
				e.Kind = kind.File
			}
			if err := validate(e); err != nil {
				return nil, fmt.Errorf("catalog: %s entry %q: %w", cat.ID, e.Name, err)
			}
		}
	}
	return &c, nil
}

func validate(e *Entry) error {
	switch {
	case e.Name == "" || e.Desc == "" || e.Path == "":
		return fmt.Errorf("name, desc and path are required")
	case !strings.HasPrefix(e.Path, "/") && !strings.HasPrefix(e.Path, "~/") && !strings.HasPrefix(e.Path, "$"):
		return fmt.Errorf("path must be absolute, ~/ or $XDG_*: %s", e.Path)
	case e.Kind == kind.Folder || !e.Kind.Valid():
		return fmt.Errorf("invalid kind %q", e.Kind)
	case (e.Kind == kind.Glob) != strings.ContainsAny(e.Path, "*?["):
		return fmt.Errorf("glob kind and glob characters must go together")
	case e.Defaults != "" && (e.Kind != kind.File || !strings.HasPrefix(e.Defaults, "/")):
		return fmt.Errorf("defaults needs a file entry and an absolute path")
	}
	for _, f := range e.Distros {
		if _, ok := distro.ParseFamily(string(f)); !ok {
			return fmt.Errorf("unknown distro %q", f)
		}
	}
	for f := range e.Hints {
		if _, ok := distro.ParseFamily(f); !ok {
			return fmt.Errorf("unknown distro %q in hints", f)
		}
	}
	return nil
}
