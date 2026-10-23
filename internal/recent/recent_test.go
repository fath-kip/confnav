package recent

import (
	"path/filepath"
	"testing"
	"time"
)

func paths(l *List) []string {
	var out []string
	for _, it := range l.Items {
		out = append(out, it.Path)
	}
	return out
}

func TestAddOrderDedupLimit(t *testing.T) {
	l := Load(filepath.Join(t.TempDir(), "recent.json"), 3)
	now := time.Now()
	for _, p := range []string{"/a", "/b", "/c", "/a", "/d"} {
		l.Add(p, now)
	}
	got := paths(l)
	want := []string{"/d", "/a", "/c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	l.Remove("/a")
	if len(l.Items) != 2 {
		t.Errorf("Remove() left %v", paths(l))
	}
}

func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "recent.json")
	l := Load(path, 10)
	at := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)
	l.Add("/etc/hosts", at)
	if err := l.Save(); err != nil {
		t.Fatal(err)
	}
	got := Load(path, 10)
	if len(got.Items) != 1 || got.Items[0].Path != "/etc/hosts" || !got.Items[0].Opened.Equal(at) {
		t.Errorf("round trip = %+v", got.Items)
	}
}
