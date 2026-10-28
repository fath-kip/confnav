package tree

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fath-kip/confnav/internal/catalog"
	"github.com/fath-kip/confnav/internal/distro"
	"github.com/fath-kip/confnav/internal/kind"
	"github.com/fath-kip/confnav/internal/library"
	"github.com/fath-kip/confnav/internal/recent"
)

const testCatalog = `
[[category]]
id = "dns"
name = "DNS"
  [[category.entry]]
  name = "hosts"
  desc = "Static hosts"
  path = "/etc/hosts"
  [[category.entry]]
  name = "resolved.conf"
  desc = "resolved"
  path = "/etc/systemd/resolved.conf"
  hint = "restart resolved"
  [[category.entry]]
  name = "apt only"
  desc = "debian thing"
  path = "/etc/apt/x"
  distros = ["debian"]

[[category]]
id = "pkg"
name = "Packages"
  [[category.entry]]
  name = "sources.list"
  desc = "apt"
  path = "/etc/apt/sources.list"
  distros = ["debian"]
`

func source(t *testing.T) Source {
	t.Helper()
	c, err := catalog.Parse([]byte(testCatalog))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	return Source{
		Catalog: c,
		Family:  distro.Arch,
		Library: library.New(filepath.Join(dir, "library.json")),
		Recent:  recent.Load(filepath.Join(dir, "recent.json"), 10),
	}
}

func build(s Source) []*Node {
	roots, _ := Build(s)
	return roots
}

func names(nodes []*Node) []string {
	var out []string
	for _, n := range nodes {
		out = append(out, n.Name)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBuildFiltersAndOrders(t *testing.T) {
	s := source(t)
	roots := build(s)
	// Packages has no arch entries, so it disappears.
	if got := names(roots); !equal(got, []string{"Recent", "My Configs", "DNS"}) {
		t.Fatalf("roots = %v", got)
	}
	dns := roots[2]
	if got := names(dns.Children); !equal(got, []string{"hosts", "resolved.conf"}) {
		t.Errorf("DNS children = %v", got)
	}
	if dns.Children[1].Hint != "restart resolved" || dns.Children[0].Parent != dns {
		t.Error("hint or parent not set")
	}
	if c, ok := dns.Container(); !ok || c != "cat:dns" {
		t.Errorf("Container() = %q %v", c, ok)
	}
	if _, ok := roots[0].Container(); ok {
		t.Error("Recent must not accept inserts")
	}
}

func TestBuildAppliesLibrary(t *testing.T) {
	s := source(t)
	lib := s.Library
	lib.Hide("dns:/etc/hosts")
	lib.SetOverride("cat:dns", "Name resolution", "")
	lib.Insert("cat:dns", &library.Node{ID: "u:1", Kind: kind.File, Name: "mine", Path: "/etc/mine.conf"})
	lib.Insert(lib.Folders[0].ID, &library.Node{ID: "u:2", Kind: kind.File, Name: "resolved", Path: "/etc/systemd/resolved.conf"})

	roots := build(s)
	dns := roots[2]
	if dns.Name != "Name resolution" {
		t.Errorf("override not applied: %q", dns.Name)
	}
	if got := names(dns.Children); !equal(got, []string{"resolved.conf", "mine"}) {
		t.Errorf("DNS children = %v", got)
	}
	if dns.Children[1].Origin != User {
		t.Error("extra must have User origin")
	}
	// A user entry pointing at a catalog file inherits its hint and description.
	mine := roots[1].Children[0]
	if mine.Hint != "restart resolved" || mine.Desc != "resolved" {
		t.Errorf("user entry did not inherit catalog info: %+v", mine)
	}
	if mine.Crumb() != "My Configs" {
		t.Errorf("Crumb() = %q", mine.Crumb())
	}

	lib.Hide("cat:dns")
	if got := names(build(s)); !equal(got, []string{"Recent", "My Configs"}) {
		t.Errorf("hidden category still shown: %v", got)
	}
}

func TestBuildRecent(t *testing.T) {
	s := source(t)
	s.Recent.Add("/etc/unknown.conf", time.Now())
	s.Recent.Add("/etc/systemd/resolved.conf", time.Now())
	r := build(s)[0]
	if got := names(r.Children); !equal(got, []string{"resolved.conf", "unknown.conf"}) {
		t.Fatalf("recent = %v", got)
	}
	if r.Children[0].Desc != "resolved" || r.Children[0].Hint != "restart resolved" {
		t.Errorf("recent item missing catalog info: %+v", r.Children[0])
	}
}

func TestLoadDirAndGlob(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.conf"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), nil, 0o644)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)

	d := &Node{ID: "x", Kind: kind.Dir, Path: dir, Hint: "h"}
	Load(d)
	if !d.Loaded || d.LoadErr != nil || !equal(names(d.Children), []string{"sub", "a.conf", "b.txt"}) {
		t.Fatalf("Load(dir) = %v err=%v", names(d.Children), d.LoadErr)
	}
	if d.Children[0].Kind != kind.Dir || d.Children[1].Origin != Dynamic || d.Children[1].Hint != "h" {
		t.Errorf("unexpected child %+v", d.Children[1])
	}

	g := &Node{ID: "g", Kind: kind.Glob, Path: filepath.Join(dir, "*.conf")}
	Load(g)
	if !equal(names(g.Children), []string{"a.conf"}) {
		t.Errorf("Load(glob) = %v", names(g.Children))
	}

	missing := &Node{ID: "m", Kind: kind.Dir, Path: filepath.Join(dir, "nope")}
	Load(missing)
	if missing.LoadErr == nil {
		t.Error("Load(missing dir) should record an error")
	}
}

func TestEntries(t *testing.T) {
	s := source(t)
	s.Recent.Add("/etc/hosts", time.Now())
	got := names(Entries(build(s)))
	if !equal(got, []string{"hosts", "resolved.conf"}) {
		t.Errorf("Entries() = %v", got)
	}
}

func TestHideMissing(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "present.conf")
	os.WriteFile(present, nil, 0o644)
	c, err := catalog.Parse([]byte(`
[[category]]
id = "a"
name = "A"
  [[category.entry]]
  name = "present"
  desc = "d"
  path = "` + present + `"
  [[category.entry]]
  name = "gone"
  desc = "d"
  path = "` + dir + `/gone.conf"
  [[category.entry]]
  name = "no match"
  desc = "d"
  path = "` + dir + `/*.nothing"
  kind = "glob"
[[category]]
id = "b"
name = "B"
  [[category.entry]]
  name = "gone too"
  desc = "d"
  path = "` + dir + `/gone2.conf"
`))
	if err != nil {
		t.Fatal(err)
	}
	s := source(t)
	s.Catalog = c
	s.Library.Insert(s.Library.Folders[0].ID, &library.Node{ID: "u:1", Kind: kind.File, Name: "mine", Path: dir + "/not-yet.conf"})
	s.Recent.Add(dir+"/gone.conf", time.Now())
	s.HideMissing = true

	roots, hidden := Build(s)
	if hidden != 3 {
		t.Errorf("hidden = %d, want 3", hidden)
	}
	if got := names(roots); !equal(got, []string{"Recent", "My Configs", "A"}) {
		t.Fatalf("roots = %v (category B is empty and must go)", got)
	}
	if got := names(roots[2].Children); !equal(got, []string{"present"}) {
		t.Errorf("A = %v", got)
	}
	if len(roots[1].Children) != 1 || len(roots[0].Children) != 1 {
		t.Error("user entries and recent items must not be filtered")
	}
	if _, hidden := Build(Source{Catalog: c, Library: s.Library, Recent: s.Recent}); hidden != 0 {
		t.Error("nothing is hidden without HideMissing")
	}
}
