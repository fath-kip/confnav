// Package tree turns the configured structure and the recent list into the
// single tree the UI shows.
package tree

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/fath-kip/confnav/internal/catalog"
	"github.com/fath-kip/confnav/internal/config"
	"github.com/fath-kip/confnav/internal/fsinfo"
	"github.com/fath-kip/confnav/internal/kind"
	"github.com/fath-kip/confnav/internal/recent"
)

// Origin tells where a node comes from, which decides what can be done with it.
type Origin int

const (
	Config  Origin = iota // from [category_structure]: fully editable
	Recent                // the Recent folder and its items
	Dynamic               // directory contents or glob matches, read from disk
)

// RecentID is the ID of the Recent folder.
const RecentID = "recent"

// Node is one row of the tree.
type Node struct {
	ID       string
	Kind     kind.Kind
	Name     string
	Desc     string
	Path     string // as written in the config (may contain ~)
	Hint     string
	Defaults string // vendor copy to start from when Path is missing
	Origin   Origin
	Opened   time.Time // Recent items only
	Parent   *Node
	Children []*Node

	// Folder and Entry are what a Config node was built from; edits go
	// through them and are then saved.
	Folder *config.Folder
	Entry  *config.Entry

	Loaded  bool  // Dir and Glob: children were read from disk
	LoadErr error // Dir and Glob: why reading failed
}

// Expandable reports whether the node has (or may have) children.
func (n *Node) Expandable() bool { return n.Kind != kind.File }

// FullPath returns the expanded path.
func (n *Node) FullPath() string { return fsinfo.Expand(n.Path) }

// Container returns the config folder new nodes can be inserted into, if
// this node is a folder that accepts them.
func (n *Node) Container() (*config.Folder, bool) {
	if n.Kind != kind.Folder || n.Origin != Config {
		return nil, false
	}
	return n.Folder, true
}

// ParentFolder returns the config folder holding n (nil: the top level).
func (n *Node) ParentFolder() *config.Folder {
	if n.Parent == nil {
		return nil
	}
	return n.Parent.Folder
}

// Own reports whether n is a folder the user made, as opposed to a
// built-in category or one a scan created.
func (n *Node) Own() bool {
	return n.Folder != nil && n.Folder.Catalog == "" && !n.Folder.Scanned
}

// FolderNode returns the nearest folder at or above n.
func (n *Node) FolderNode() *Node {
	for f := n; f != nil; f = f.Parent {
		if f.Kind == kind.Folder {
			return f
		}
	}
	return nil
}

// Crumb returns the names of n's ancestors, e.g. "My Configs › Work".
func (n *Node) Crumb() string {
	var names []string
	for p := n.Parent; p != nil; p = p.Parent {
		names = append([]string{p.Name}, names...)
	}
	return strings.Join(names, " › ")
}

// Source is everything Build needs.
type Source struct {
	Structure *config.Structure
	Recent    *recent.List
	// HideMissing leaves out entries of categories that don't exist on disk.
	HideMissing bool
}

// Build returns the top-level nodes: Recent, then the categories and the
// top-level entries of the structure. It also returns how many entries
// HideMissing left out.
func Build(s Source) ([]*Node, int) {
	b := builder{known: map[string]*Node{}, ids: map[string]int{}, hideMissing: s.HideMissing}
	roots := []*Node{nil} // Recent goes first but needs the known map
	for _, f := range s.Structure.Categories {
		if n := b.folder(f, nil); n != nil {
			roots = append(roots, n)
		}
	}
	for _, e := range s.Structure.Entries {
		if n := b.entry(e, nil); n != nil {
			roots = append(roots, n)
		}
	}

	rn := &Node{ID: RecentID, Kind: kind.Folder, Name: "Recent", Origin: Recent}
	for _, it := range s.Recent.Items {
		n := &Node{ID: "r:" + it.Path, Kind: kind.File, Name: filepath.Base(it.Path),
			Path: it.Path, Origin: Recent, Opened: it.Opened, Parent: rn}
		if k, ok := b.known[it.Path]; ok {
			n.Desc, n.Hint, n.Defaults = k.Desc, k.Hint, k.Defaults
		}
		rn.Children = append(rn.Children, n)
	}
	roots[0] = rn
	return roots, b.hidden
}

type builder struct {
	known       map[string]*Node // expanded path → first entry, for Recent items
	ids         map[string]int   // to make repeated IDs unique
	hideMissing bool
	hidden      int
}

// id makes base unique, so two folders of the same name stay apart.
func (b *builder) id(base string) string {
	b.ids[base]++
	if n := b.ids[base]; n > 1 {
		return fmt.Sprintf("%s#%d", base, n)
	}
	return base
}

// folder converts f; a built-in category that only had missing files is
// left out when they are hidden.
func (b *builder) folder(f *config.Folder, parent *Node) *Node {
	base := "f:" + f.Name
	switch {
	case parent == nil && f.Catalog != "":
		base = catalog.CategoryID(f.Catalog)
	case parent != nil:
		base = parent.ID + "/" + f.Name
	}
	n := &Node{ID: b.id(base), Kind: kind.Folder, Name: f.Name, Origin: Config, Parent: parent, Folder: f}
	before := b.hidden
	for _, c := range f.Folders {
		if cn := b.folder(c, n); cn != nil {
			n.Children = append(n.Children, cn)
		}
	}
	for _, e := range f.Entries {
		if en := b.entry(e, n); en != nil {
			n.Children = append(n.Children, en)
		}
	}
	if len(n.Children) == 0 && b.hidden > before && !n.Own() {
		return nil
	}
	return n
}

func (b *builder) entry(e *config.Entry, parent *Node) *Node {
	pid := ""
	if parent != nil {
		pid = parent.ID
	}
	n := &Node{ID: b.id(pid + ":" + e.Path), Kind: e.Kind, Name: e.Name, Desc: e.Desc, Path: e.Path,
		Hint: e.Hint, Defaults: e.Defaults, Origin: Config, Parent: parent, Entry: e}
	if _, ok := b.known[n.FullPath()]; !ok {
		b.known[n.FullPath()] = n
	}
	// Files you added to your own folders or the top level stay visible.
	own := parent == nil || parent.Own()
	if b.hideMissing && !own && missing(n) {
		b.hidden++
		return nil
	}
	return n
}

// missing reports whether an entry has nothing on disk.
func missing(n *Node) bool {
	if n.Kind == kind.Glob {
		m, _ := fsinfo.Glob(n.FullPath())
		return len(m) == 0
	}
	return fsinfo.Stat(n.FullPath()).State == fsinfo.Missing
}

// Load reads the children of a Dir or Glob node from disk.
func Load(n *Node) {
	var kids []fsinfo.Child
	var err error
	switch n.Kind {
	case kind.Dir:
		kids, err = fsinfo.ListDir(n.FullPath())
	case kind.Glob:
		kids, err = fsinfo.Glob(n.FullPath())
	default:
		return
	}
	n.Loaded, n.LoadErr, n.Children = true, err, nil
	for _, c := range kids {
		k := kind.File
		if c.IsDir {
			k = kind.Dir
		}
		n.Children = append(n.Children, &Node{
			ID: n.ID + "/" + filepath.Base(c.Path), Kind: k, Name: filepath.Base(c.Path),
			Path: c.Path, Hint: n.Hint, Origin: Dynamic, Parent: n,
		})
	}
}

// Entries returns every non-folder node reachable without touching the
// disk, skipping Recent (its items are duplicates). Used by search.
func Entries(roots []*Node) []*Node {
	var out []*Node
	var walk func([]*Node)
	walk = func(nodes []*Node) {
		for _, n := range nodes {
			switch {
			case n.Origin == Recent || n.Origin == Dynamic:
			case n.Kind == kind.Folder:
				walk(n.Children)
			default:
				out = append(out, n)
			}
		}
	}
	walk(roots)
	return out
}
