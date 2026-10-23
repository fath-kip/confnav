// Package tree merges the catalog, the user's library and the recent list
// into the single tree the UI shows.
package tree

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/fath-kip/confnav/internal/catalog"
	"github.com/fath-kip/confnav/internal/distro"
	"github.com/fath-kip/confnav/internal/fsinfo"
	"github.com/fath-kip/confnav/internal/kind"
	"github.com/fath-kip/confnav/internal/library"
	"github.com/fath-kip/confnav/internal/recent"
)

// Origin tells where a node comes from, which decides what can be done with it.
type Origin int

const (
	Builtin Origin = iota // from the catalog: can be hidden and renamed
	User                  // from the library: fully editable
	Recent                // the Recent folder and its items
	Dynamic               // directory contents or glob matches, read from disk
)

// RecentID is the ID of the Recent folder.
const RecentID = "recent"

// AppConfigID is the ID of the entry for confnav's own config.toml.
const AppConfigID = "app:config"

// Node is one row of the tree.
type Node struct {
	ID       string
	Kind     kind.Kind
	Name     string
	Desc     string
	Path     string // as written in the catalog or by the user (may contain ~)
	Hint     string
	Defaults string // vendor copy to start from when Path is missing
	Origin   Origin
	Opened   time.Time // Recent items only
	Parent   *Node
	Children []*Node

	Loaded  bool  // Dir and Glob: children were read from disk
	LoadErr error // Dir and Glob: why reading failed
}

// Expandable reports whether the node has (or may have) children.
func (n *Node) Expandable() bool { return n.Kind != kind.File }

// FullPath returns the expanded path.
func (n *Node) FullPath() string { return fsinfo.Expand(n.Path) }

// Container returns the library container ID new nodes can be inserted
// into, if this node is a folder that accepts them.
func (n *Node) Container() (string, bool) {
	if n.Kind != kind.Folder || (n.Origin != Builtin && n.Origin != User) {
		return "", false
	}
	return n.ID, true
}

// Folder returns the nearest folder at or above n.
func (n *Node) Folder() *Node {
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
	Catalog *catalog.Catalog
	Family  distro.Family
	Library *library.Library
	Recent  *recent.List
	// AppConfig is the path of confnav's config.toml, listed at the top level.
	AppConfig string
	// HideMissing leaves out built-in entries that don't exist on disk.
	HideMissing bool
}

// Build returns the top-level nodes: Recent, the user's own nodes, the
// catalog categories for the current distro family and confnav's own
// settings. It also returns how many built-ins HideMissing left out.
func Build(s Source) ([]*Node, int) {
	hidden := 0
	known := map[string]*Node{} // expanded path → first built-in node, for descriptions and hints
	var cats []*Node
	for _, cat := range s.Catalog.Categories {
		cid := catalog.CategoryID(cat.ID)
		if s.Library.IsHidden(cid) {
			continue
		}
		cn := &Node{ID: cid, Kind: kind.Folder, Name: cat.Name, Origin: Builtin}
		applyOverride(cn, s.Library)
		for _, e := range cat.Entries {
			id := catalog.EntryID(cat.ID, e.Path)
			if !e.For(s.Family) || s.Library.IsHidden(id) {
				continue
			}
			n := &Node{ID: id, Kind: e.Kind, Name: e.Name, Desc: e.Desc, Path: e.Path,
				Hint: e.HintFor(s.Family), Defaults: e.Defaults, Origin: Builtin, Parent: cn}
			if _, ok := known[n.FullPath()]; !ok {
				known[n.FullPath()] = n
			}
			if s.HideMissing && missing(n) {
				hidden++
				continue
			}
			applyOverride(n, s.Library)
			cn.Children = append(cn.Children, n)
		}
		cats = append(cats, cn)
	}
	for _, cn := range cats {
		for _, u := range s.Library.ExtrasFor(cn.ID) {
			cn.Children = append(cn.Children, fromUser(u, cn, known))
		}
	}

	rn := &Node{ID: RecentID, Kind: kind.Folder, Name: "Recent", Origin: Recent}
	for _, it := range s.Recent.Items {
		n := &Node{ID: "r:" + it.Path, Kind: kind.File, Name: filepath.Base(it.Path),
			Path: it.Path, Origin: Recent, Opened: it.Opened, Parent: rn}
		if k, ok := known[it.Path]; ok {
			n.Desc, n.Hint, n.Defaults = k.Desc, k.Hint, k.Defaults
		}
		rn.Children = append(rn.Children, n)
	}

	roots := []*Node{rn}
	for _, f := range s.Library.Folders {
		roots = append(roots, fromUser(f, nil, known))
	}
	for _, cn := range cats {
		if len(cn.Children) > 0 {
			roots = append(roots, cn)
		}
	}
	if s.AppConfig != "" && !s.Library.IsHidden(AppConfigID) {
		n := &Node{ID: AppConfigID, Kind: kind.File, Name: "confnav settings",
			Desc: "Editor, key bindings, mouse, backups", Path: s.AppConfig,
			Hint: "changes apply as soon as you save", Origin: Builtin}
		applyOverride(n, s.Library)
		roots = append(roots, n)
	}
	return roots, hidden
}

// missing reports whether a built-in entry has nothing on disk.
func missing(n *Node) bool {
	if n.Kind == kind.Glob {
		m, _ := fsinfo.Glob(n.FullPath())
		return len(m) == 0
	}
	return fsinfo.Stat(n.FullPath()).State == fsinfo.Missing
}

func applyOverride(n *Node, lib *library.Library) {
	o, ok := lib.Overrides[n.ID]
	if !ok {
		return
	}
	if o.Name != "" {
		n.Name = o.Name
	}
	if o.Desc != "" {
		n.Desc = o.Desc
	}
}

func fromUser(u *library.Node, parent *Node, known map[string]*Node) *Node {
	n := &Node{ID: u.ID, Kind: u.Kind, Name: u.Name, Desc: u.Desc, Path: u.Path,
		Origin: User, Parent: parent}
	if k, ok := known[n.FullPath()]; ok && u.Kind != kind.Folder {
		n.Hint, n.Defaults = k.Hint, k.Defaults
		if n.Desc == "" {
			n.Desc = k.Desc
		}
	}
	for _, c := range u.Children {
		n.Children = append(n.Children, fromUser(c, n, known))
	}
	return n
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
