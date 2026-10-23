// Package library stores what the user changed on top of the built-in
// catalog: their own folders and entries, hidden built-ins and renames.
//
// The catalog itself is never copied into the library, so catalog updates in
// new releases reach users automatically.
package library

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/fath-kip/confnav/internal/catalog"
	"github.com/fath-kip/confnav/internal/kind"
	"github.com/fath-kip/confnav/internal/xdg"
)

// Root is the container ID for the top level of the tree.
const Root = ""

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
	// directories added at the top level. The JSON name predates the latter.
	Folders []*Node `json:"folders"`
	// Extras are user nodes added inside built-in categories.
	Extras []Extra `json:"extras,omitempty"`
	// Hidden lists built-in category and entry IDs the user removed.
	Hidden []string `json:"hidden,omitempty"`
	// Overrides holds user renames of built-in categories and entries.
	Overrides map[string]Override `json:"overrides,omitempty"`

	path string
}

// Path returns the location of library.json.
func Path() string { return filepath.Join(xdg.DataDir(), "library.json") }

// New returns the library a first-time user starts with.
func New(path string) *Library {
	return &Library{
		Version: 1,
		Folders: []*Node{{ID: NewID(), Kind: kind.Folder, Name: "My Configs"}},
		path:    path,
	}
}

// Load reads library.json, or returns New(path) if it does not exist. A
// corrupt file is an error rather than silently replaced, so the user's
// data is never overwritten.
func Load(path string) (*Library, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return New(path), nil
	}
	if err != nil {
		return nil, err
	}
	l := &Library{}
	if err := json.Unmarshal(data, l); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	l.path = path
	return l, nil
}

// Save writes the library atomically.
func (l *Library) Save() error {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return xdg.WriteFileAtomic(l.path, append(data, '\n'), 0o644)
}

// NewID returns a random ID for a user node.
func NewID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return "u:" + hex.EncodeToString(b)
}

// IsHidden reports whether a built-in ID was hidden.
func (l *Library) IsHidden(id string) bool { return slices.Contains(l.Hidden, id) }

// Hide hides a built-in category or entry.
func (l *Library) Hide(id string) {
	if !l.IsHidden(id) {
		l.Hidden = append(l.Hidden, id)
	}
}

// RestoreHidden un-hides every built-in item and returns how many there were.
func (l *Library) RestoreHidden() int {
	n := len(l.Hidden)
	l.Hidden = nil
	return n
}

// SetOverride renames a built-in item. Empty fields keep the built-in text.
func (l *Library) SetOverride(id, name, desc string) {
	if l.Overrides == nil {
		l.Overrides = map[string]Override{}
	}
	if name == "" && desc == "" {
		delete(l.Overrides, id)
		return
	}
	l.Overrides[id] = Override{Name: name, Desc: desc}
}

// Find returns the user node with the given ID.
func (l *Library) Find(id string) *Node {
	n, _, _ := l.locate(id)
	return n
}

// Insert adds n to a container: Root, a built-in category ID or a user folder ID.
func (l *Library) Insert(container string, n *Node) error {
	switch {
	case container == Root:
		l.Folders = append(l.Folders, n)
	case isCategory(container):
		for i := range l.Extras {
			if l.Extras[i].Category == container {
				l.Extras[i].Nodes = append(l.Extras[i].Nodes, n)
				return nil
			}
		}
		l.Extras = append(l.Extras, Extra{Category: container, Nodes: []*Node{n}})
	default:
		parent := l.Find(container)
		if parent == nil || parent.Kind != kind.Folder {
			return fmt.Errorf("folder %q not found", container)
		}
		parent.Children = append(parent.Children, n)
	}
	return nil
}

// Remove deletes a user node and everything below it.
func (l *Library) Remove(id string) bool {
	_, list, i := l.locate(id)
	if list == nil {
		return false
	}
	*list = slices.Delete(*list, i, i+1)
	return true
}

// Move puts a user node into another container.
func (l *Library) Move(id, container string) error {
	n, list, i := l.locate(id)
	if n == nil {
		return fmt.Errorf("entry %q not found", id)
	}
	if container == id || (n.Kind == kind.Folder && contains(n, container)) {
		return errors.New("cannot move a folder into itself")
	}
	*list = slices.Delete(*list, i, i+1)
	return l.Insert(container, n)
}

// Clone deep-copies a node with fresh IDs.
func Clone(n *Node) *Node {
	c := *n
	c.ID = NewID()
	c.Children = nil
	for _, ch := range n.Children {
		c.Children = append(c.Children, Clone(ch))
	}
	return &c
}

// locate finds a node and the slice holding it.
func (l *Library) locate(id string) (*Node, *[]*Node, int) {
	if n, list, i := find(&l.Folders, id); n != nil {
		return n, list, i
	}
	for e := range l.Extras {
		if n, list, i := find(&l.Extras[e].Nodes, id); n != nil {
			return n, list, i
		}
	}
	return nil, nil, -1
}

// ExtrasFor returns the user nodes added to a built-in category.
func (l *Library) ExtrasFor(category string) []*Node {
	for _, e := range l.Extras {
		if e.Category == category {
			return e.Nodes
		}
	}
	return nil
}

func find(list *[]*Node, id string) (*Node, *[]*Node, int) {
	for i, n := range *list {
		if n.ID == id {
			return n, list, i
		}
		if n, l, j := find(&n.Children, id); n != nil {
			return n, l, j
		}
	}
	return nil, nil, -1
}

func contains(folder *Node, id string) bool {
	for _, c := range folder.Children {
		if c.ID == id || contains(c, id) {
			return true
		}
	}
	return false
}

func isCategory(container string) bool {
	return len(container) > 4 && container[:4] == catalog.CategoryID("")
}
