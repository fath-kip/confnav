package library

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fath-kip/confnav/internal/kind"
)

func folder(name string) *Node { return &Node{ID: NewID(), Kind: kind.Folder, Name: name} }
func file(name, path string) *Node {
	return &Node{ID: NewID(), Kind: kind.File, Name: name, Path: path}
}

func TestLoadMissingCreatesDefault(t *testing.T) {
	l, err := Load(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Folders) != 1 || l.Folders[0].Name != "My Configs" {
		t.Errorf("default folders = %+v", l.Folders)
	}
}

func TestLoadCorruptIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	os.WriteFile(path, []byte("{not json"), 0o644)
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error for corrupt library")
	}
	if data, _ := os.ReadFile(path); string(data) != "{not json" {
		t.Error("corrupt library was modified")
	}
}

func TestInsertRemoveMove(t *testing.T) {
	l := New(filepath.Join(t.TempDir(), "library.json"))
	mine := l.Folders[0]
	work := folder("Work")
	hosts := file("hosts", "/etc/hosts")

	must(t, l.Insert(mine.ID, work))
	must(t, l.Insert(work.ID, hosts))
	if l.Find(hosts.ID) != hosts {
		t.Fatal("Find() did not find nested node")
	}

	// Into a built-in category and back out again.
	must(t, l.Move(hosts.ID, "cat:dns"))
	if len(work.Children) != 0 || len(l.ExtrasFor("cat:dns")) != 1 {
		t.Fatalf("move to category failed: work=%v extras=%v", work.Children, l.Extras)
	}
	must(t, l.Move(hosts.ID, mine.ID))
	if len(l.ExtrasFor("cat:dns")) != 0 || len(mine.Children) != 2 {
		t.Fatalf("move out of category failed: extras=%v mine=%v", l.Extras, mine.Children)
	}

	if err := l.Move(mine.ID, work.ID); err == nil {
		t.Error("moving a folder into its own child must fail")
	}
	if err := l.Move(mine.ID, mine.ID); err == nil {
		t.Error("moving a folder into itself must fail")
	}
	must(t, l.Move(work.ID, Root))
	if len(l.Folders) != 2 {
		t.Errorf("work folder not at top level: %v", l.Folders)
	}

	// Files may live at the top level too.
	must(t, l.Move(hosts.ID, Root))
	if len(l.Folders) != 3 || l.Folders[2] != hosts {
		t.Fatalf("file not at top level: %v", l.Folders)
	}
	must(t, l.Move(hosts.ID, mine.ID))

	if !l.Remove(mine.ID) || l.Find(hosts.ID) != nil {
		t.Error("Remove() must drop the folder and its children")
	}
	if l.Remove("u:missing") {
		t.Error("Remove() of unknown ID returned true")
	}
}

func TestHiddenAndOverrides(t *testing.T) {
	l := New(filepath.Join(t.TempDir(), "library.json"))
	l.Hide("dns:/etc/hosts")
	l.Hide("dns:/etc/hosts")
	if !l.IsHidden("dns:/etc/hosts") || len(l.Hidden) != 1 {
		t.Errorf("Hidden = %v", l.Hidden)
	}
	if n := l.RestoreHidden(); n != 1 || l.IsHidden("dns:/etc/hosts") {
		t.Errorf("RestoreHidden() = %d, hidden=%v", n, l.Hidden)
	}
	l.SetOverride("cat:dns", "Name resolution", "")
	if l.Overrides["cat:dns"].Name != "Name resolution" {
		t.Error("override not stored")
	}
	l.SetOverride("cat:dns", "", "")
	if _, ok := l.Overrides["cat:dns"]; ok {
		t.Error("empty override must be deleted")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "library.json")
	l := New(path)
	must(t, l.Insert(l.Folders[0].ID, file("hosts", "/etc/hosts")))
	must(t, l.Insert("cat:ssh", file("work", "~/.ssh/work")))
	l.Hide("dns:/etc/resolv.conf")
	l.SetOverride("cat:dns", "Resolver", "")
	must(t, l.Save())

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Folders[0].Children) != 1 || got.Folders[0].Children[0].Path != "/etc/hosts" ||
		len(got.ExtrasFor("cat:ssh")) != 1 || !got.IsHidden("dns:/etc/resolv.conf") ||
		got.Overrides["cat:dns"].Name != "Resolver" {
		t.Errorf("round trip lost data: %+v", got)
	}
}

func TestClone(t *testing.T) {
	f := folder("A")
	f.Children = []*Node{file("x", "/x")}
	c := Clone(f)
	if c.ID == f.ID || c.Children[0].ID == f.Children[0].ID || c.Children[0].Path != "/x" {
		t.Errorf("Clone() = %+v", c)
	}
	c.Children[0].Name = "changed"
	if f.Children[0].Name != "x" {
		t.Error("Clone() shares children")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
