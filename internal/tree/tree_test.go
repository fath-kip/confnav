package tree

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fath-kip/confnav/internal/config"
	"github.com/fath-kip/confnav/internal/kind"
	"github.com/fath-kip/confnav/internal/recent"
)

func source(t *testing.T) Source {
	t.Helper()
	return Source{
		Structure: &config.Structure{Categories: []*config.Folder{
			{Name: "My Configs"},
			{Catalog: "dns", Name: "DNS", Entries: []*config.Entry{
				{Name: "hosts", Desc: "Static hosts", Path: "/etc/hosts", Kind: kind.File},
				{Name: "resolved.conf", Desc: "resolved", Path: "/etc/systemd/resolved.conf", Kind: kind.File, Hint: "restart resolved"},
			}},
		}},
		Recent: recent.Load(filepath.Join(t.TempDir(), "recent.json"), 10),
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

func TestBuildFollowsStructure(t *testing.T) {
	s := source(t)
	dns := s.Structure.Categories[1]
	dns.Folders = []*config.Folder{{Name: "VPN", Entries: []*config.Entry{{Name: "wg", Path: "/etc/wg.conf", Kind: kind.File}}}}
	s.Structure.Entries = []*config.Entry{{Name: "top", Path: "/etc/top.conf", Kind: kind.File}}

	roots := build(s)
	if got := names(roots); !equal(got, []string{"Recent", "My Configs", "DNS", "top"}) {
		t.Fatalf("roots = %v", got)
	}
	d := roots[2]
	if got := names(d.Children); !equal(got, []string{"VPN", "hosts", "resolved.conf"}) {
		t.Errorf("DNS children = %v", got)
	}
	if d.ID != "cat:dns" || d.Children[0].ID != "cat:dns/VPN" || d.Children[1].ID != "cat:dns:/etc/hosts" {
		t.Errorf("IDs = %q %q %q", d.ID, d.Children[0].ID, d.Children[1].ID)
	}
	if d.Children[2].Hint != "restart resolved" || d.Children[1].Parent != d || d.Children[1].Entry != dns.Entries[0] {
		t.Error("hint, parent or entry not set")
	}
	if c, ok := d.Container(); !ok || c != dns {
		t.Errorf("Container() = %v %v", c, ok)
	}
	if _, ok := roots[0].Container(); ok {
		t.Error("Recent must not accept inserts")
	}
	if roots[1].ID != "f:My Configs" || !roots[1].Own() || d.Own() || !d.Children[0].Own() {
		t.Error("Own() should mark folders the user made")
	}
	if d.Children[0].Children[0].ParentFolder() != dns.Folders[0] || roots[3].ParentFolder() != nil {
		t.Error("ParentFolder() wrong")
	}
	if got := d.Children[0].Children[0].Crumb(); got != "DNS › VPN" {
		t.Errorf("Crumb() = %q", got)
	}
}

func TestRepeatedNamesGetUniqueIDs(t *testing.T) {
	s := source(t)
	s.Structure.Categories = append(s.Structure.Categories, &config.Folder{Name: "My Configs"})
	roots := build(s)
	if roots[1].ID == roots[3].ID {
		t.Errorf("two folders share the ID %q", roots[1].ID)
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
		t.Errorf("recent item missing the listed entry's info: %+v", r.Children[0])
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
	s := source(t)
	s.Structure.Categories = []*config.Folder{
		{Name: "My Configs", Entries: []*config.Entry{{Name: "mine", Path: dir + "/not-yet.conf", Kind: kind.File}}},
		{Catalog: "a", Name: "A", Entries: []*config.Entry{
			{Name: "present", Path: present, Kind: kind.File},
			{Name: "gone", Path: dir + "/gone.conf", Kind: kind.File},
			{Name: "no match", Path: dir + "/*.nothing", Kind: kind.Glob},
		}},
		{Catalog: "b", Name: "B", Entries: []*config.Entry{{Name: "gone too", Path: dir + "/gone2.conf", Kind: kind.File}}},
		{Name: "Empty"},
	}
	s.Recent.Add(dir+"/gone.conf", time.Now())
	s.HideMissing = true

	roots, hidden := Build(s)
	if hidden != 3 {
		t.Errorf("hidden = %d, want 3", hidden)
	}
	if got := names(roots); !equal(got, []string{"Recent", "My Configs", "A", "Empty"}) {
		t.Fatalf("roots = %v (category B is empty because of hiding and must go)", got)
	}
	if got := names(roots[2].Children); !equal(got, []string{"present"}) {
		t.Errorf("A = %v", got)
	}
	if len(roots[1].Children) != 1 || len(roots[0].Children) != 1 {
		t.Error("your own entries and recent items must not be filtered")
	}
	s.HideMissing = false
	if _, hidden := Build(s); hidden != 0 {
		t.Error("nothing is hidden without HideMissing")
	}
}
