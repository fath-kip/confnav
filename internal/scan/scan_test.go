package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/fath-kip/confnav/internal/catalog"
	"github.com/fath-kip/confnav/internal/config"
	"github.com/fath-kip/confnav/internal/kind"
)

// fakeSystem writes files under a temp root and returns its scan roots.
func fakeSystem(t *testing.T, files map[string]string) Roots {
	t.Helper()
	root := t.TempDir()
	r := Roots{Etc: filepath.Join(root, "etc"), Boot: filepath.Join(root, "boot"), Home: filepath.Join(root, "home")}
	for name, content := range files {
		p := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func byPath(found []Found, r Roots) map[string]Found {
	out := map[string]Found{}
	root := filepath.Dir(r.Etc)
	for _, f := range found {
		rel, _ := filepath.Rel(root, f.Path)
		out[rel] = f
	}
	return out
}

func TestRunFiltersAndClassifies(t *testing.T) {
	r := fakeSystem(t, map[string]string{
		"etc/hosts":                                "127.0.0.1 localhost\n",
		"etc/pacman.conf":                          "[options]\n",
		"etc/pacman.conf.pacnew":                   "[options]\n",
		"etc/passwd-":                              "backup\n",
		"etc/NetworkManager/conf.d/dns.conf":       "[main]\n",
		"etc/systemd/timesyncd.conf":               "[Time]\n",
		"etc/systemd/logind.conf":                  "[Login]\n",
		"etc/ssh/sshd_config":                      "Port 22\n",
		"etc/ssh/ssh_host_ed25519_key":             "secret\n",
		"etc/ssl/openssl.cnf":                      "x\n",
		"etc/skel/.bashrc":                         "x\n",
		"etc/ld.so.cache":                          "bin\x00ary",
		"etc/foo/foo.conf":                         "x\n",
		"etc/motd":                                 "hi\n",
		"boot/loader/loader.conf":                  "timeout 3\n",
		"boot/loader/entries/arch.conf":            "title Arch\n",
		"boot/vmlinuz-linux":                       "kernel",
		"home/.bashrc":                             "alias ll='ls -l'\n",
		"home/.gitconfig":                          "[user]\n",
		"home/.ssh/config":                         "Host *\n",
		"home/.ssh/id_ed25519":                     "secret\n",
		"home/.config/hypr/hyprland.conf":          "monitor=,preferred\n",
		"home/.config/nvim/init.lua":               "vim.o.number = true\n",
		"home/.config/nvim/lua/plugins/a.lua":      "return {}\n",
		"home/.config/nvim/a/b/c/too-deep.lua":     "return {}\n",
		"home/.config/nvim/notes.txt":              "not settings\n",
		"home/.config/foo/Cache/x.json":            "{}\n",
		"home/.config/chromium/Default/Prefs":      "{}\n",
		"home/.config/confnav/config.toml":         "x\n",
		"home/.config/starship.toml":               "x\n",
		"home/.local/share/applications/a.desktop": "[Desktop Entry]\n",
	})
	got := byPath(Run(r), r)
	want := map[string]Found{
		"etc/hosts":                                {Category: "dns"},
		"etc/pacman.conf":                          {Category: "packages"},
		"etc/NetworkManager/conf.d/dns.conf":       {Category: "network"},
		"etc/systemd/timesyncd.conf":               {Category: "locale"},
		"etc/systemd/logind.conf":                  {Category: "systemd"},
		"etc/ssh/sshd_config":                      {Category: "ssh"},
		"etc/foo/foo.conf":                         {Category: Other, Folder: "foo"},
		"etc/motd":                                 {Category: Other},
		"boot/loader/loader.conf":                  {Category: "boot"},
		"boot/loader/entries/arch.conf":            {Category: "boot"},
		"home/.bashrc":                             {Category: "shell"},
		"home/.gitconfig":                          {Category: Apps},
		"home/.ssh/config":                         {Category: "ssh"},
		"home/.config/hypr/hyprland.conf":          {Category: "desktop"},
		"home/.config/nvim/init.lua":               {Category: Apps, Folder: "nvim"},
		"home/.config/nvim/lua/plugins/a.lua":      {Category: Apps, Folder: "nvim"},
		"home/.config/starship.toml":               {Category: "shell"},
		"home/.local/share/applications/a.desktop": {Category: "desktop"},
	}
	for path, w := range want {
		g, ok := got[path]
		if !ok {
			t.Errorf("%s not found", path)
			continue
		}
		if g.Category != w.Category || g.Folder != w.Folder {
			t.Errorf("%s = %s/%s, want %s/%s", path, g.Category, g.Folder, w.Category, w.Folder)
		}
	}
	for path := range got {
		if _, ok := want[path]; !ok {
			t.Errorf("%s should have been left out", path)
		}
	}
	if g := got["etc/hosts"].Group; g != "" {
		t.Errorf("files at the top of /etc have no group, got %q", g)
	}
	if g := got["home/.config/nvim/init.lua"].Group; g != filepath.Join(r.Home, ".config/nvim") {
		t.Errorf("group = %q", g)
	}
}

func TestRulesUseCatalogCategories(t *testing.T) {
	c, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rules {
		if c.Category(r.category) == nil {
			t.Errorf("rule %q points at unknown category %q", r.pattern, r.category)
		}
	}
}

func TestMerge(t *testing.T) {
	s := &config.Structure{
		Ignore: []string{"/etc/removed.conf"},
		Categories: []*config.Folder{
			{Catalog: "dns", Name: "DNS", Entries: []*config.Entry{{Name: "hosts", Path: "/etc/hosts", Kind: kind.File}}},
			{Catalog: "kernel", Name: "Kernel", Entries: []*config.Entry{{Name: "sysctl.d", Path: "/etc/sysctl.d", Kind: kind.Dir}}},
			{Catalog: "ssh", Name: "SSH"},
		},
	}
	found := []Found{
		{Path: "/etc/hosts", Category: "dns"},                                         // listed
		{Path: "/etc/sysctl.d/99-x.conf", Category: "kernel", Group: "/etc/sysctl.d"}, // inside a listed dir
		{Path: "/etc/removed.conf", Category: Other},                                  // removed before
		{Path: "/etc/pacman.conf", Category: "packages"},                              // category is off
		{Path: "/etc/host.conf", Category: "dns"},
		{Path: "/etc/ssh/sshd_config", Category: "ssh", Group: "/etc/ssh"},
		{Path: "/etc/foo/a.conf", Category: Other, Folder: "foo", Group: "/etc/foo"},
	}
	for i := range Compact + 1 {
		found = append(found, Found{Path: fmt.Sprintf("/etc/many/%02d.rules", i), Category: Other, Folder: "many", Group: "/etc/many"})
	}
	r := Merge(s, found)
	if r != (Result{Added: 4, Known: 3, Skipped: 1}) {
		t.Errorf("Merge = %+v", r)
	}
	dns := s.Category("dns")
	if len(dns.Entries) != 2 || dns.Entries[1].Path != "/etc/host.conf" || !dns.Entries[1].Scanned {
		t.Errorf("DNS = %+v", dns.Entries)
	}
	other := s.Category(Other)
	if other == nil || other.Name != "Other" {
		t.Fatalf("Other not created: %+v", s.Categories)
	}
	if len(other.Folders) != 1 || other.Folders[0].Name != "foo" || !other.Folders[0].Scanned {
		t.Errorf("Other folders = %+v", other.Folders)
	}
	if len(other.Entries) != 1 || other.Entries[0].Kind != kind.Dir || other.Entries[0].Path != "/etc/many" {
		t.Errorf("a crowded directory should be added as one dir entry: %+v", other.Entries)
	}
	if s.Category("packages") != nil {
		t.Error("a category the user removed must not come back")
	}
	if r := Merge(s, found); r.Added != 0 {
		t.Errorf("a second merge added %d entries again", r.Added)
	}
}
