package library

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fath-kip/confnav/internal/catalog"
	"github.com/fath-kip/confnav/internal/distro"
	"github.com/fath-kip/confnav/internal/kind"
)

func TestLoadMissingIsNil(t *testing.T) {
	l, err := Load(filepath.Join(t.TempDir(), "library.json"))
	if l != nil || err != nil {
		t.Errorf("Load(missing) = %v, %v", l, err)
	}
}

func TestLoadCorruptIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	os.WriteFile(path, []byte("{nope"), 0o644)
	if _, err := Load(path); err == nil {
		t.Error("a corrupt library must be an error, never silently replaced")
	}
}

func TestMigrate(t *testing.T) {
	c, err := catalog.Parse([]byte(`
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
id = "boot"
name = "Boot"
  [[category.entry]]
  name = "grub"
  desc = "d"
  path = "/etc/default/grub"
`))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "library.json")
	os.WriteFile(path, []byte(`{
  "version": 1,
  "folders": [
    {"id": "u:1", "kind": "folder", "name": "My Configs", "children": [
      {"id": "u:2", "kind": "file", "name": "resolved", "path": "/etc/systemd/resolved.conf"},
      {"id": "u:3", "kind": "folder", "name": "Work", "children": [
        {"id": "u:4", "kind": "dir", "name": "nm", "path": "/etc/NetworkManager"}
      ]}
    ]},
    {"id": "u:5", "kind": "file", "name": "top", "path": "/etc/top.conf"}
  ],
  "extras": [{"category": "cat:dns", "nodes": [{"id": "u:6", "kind": "file", "name": "mine", "path": "/etc/mine.conf"}]}],
  "hidden": ["dns:/etc/hosts", "cat:boot", "app:config"],
  "overrides": {"cat:dns": {"name": "Name resolution"}, "dns:/etc/systemd/resolved.conf": {"desc": "my desc"}}
}`), 0o644)
	l, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s := Migrate(l, c, distro.Arch)

	if len(s.Categories) != 2 || s.Categories[0].Name != "My Configs" || s.Categories[1].Name != "Name resolution" {
		t.Fatalf("categories = %+v (hidden Boot must be gone)", s.Categories)
	}
	mine := s.Categories[0]
	if len(mine.Entries) != 1 || mine.Entries[0].Hint != "restart resolved" || mine.Entries[0].Desc != "resolved" {
		t.Errorf("user entry should take the catalog hint and description: %+v", mine.Entries)
	}
	if len(mine.Folders) != 1 || mine.Folders[0].Name != "Work" || mine.Folders[0].Entries[0].Kind != kind.Dir {
		t.Errorf("nested folder lost: %+v", mine.Folders)
	}
	dns := s.Categories[1]
	if dns.Catalog != "dns" || len(dns.Entries) != 2 || dns.Entries[0].Name != "resolved.conf" ||
		dns.Entries[0].Desc != "my desc" || dns.Entries[1].Name != "mine" {
		t.Errorf("DNS = %+v (hosts hidden, debian entry dropped, extra appended)", dns.Entries)
	}
	if len(s.Entries) != 1 || s.Entries[0].Path != "/etc/top.conf" {
		t.Errorf("top-level entries = %+v", s.Entries)
	}
}
