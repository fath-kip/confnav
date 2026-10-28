package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fath-kip/confnav/internal/kind"
)

func TestLoadMissing(t *testing.T) {
	cfg, info, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil || info.Exists || info.HasStructure {
		t.Fatalf("Load() info=%+v err=%v", info, err)
	}
	if !reflect.DeepEqual(cfg, Default()) {
		t.Errorf("got %+v, want defaults", cfg)
	}
	if len(cfg.Keys) != len(DefaultKeys) || !cfg.ShowMissing {
		t.Errorf("defaults incomplete: %+v", cfg)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.toml")
	want := Default()
	want.Editor, want.Backup, want.Mouse, want.RecentLimit = "nvim -p", true, false, 5
	want.DateFormat, want.ShowMissing = "02.01.2006", false
	want.ScanAs = ScanRoot
	want.Keys["edit"] = KeyList{"r", "f2"}
	want.Keys["fly"] = KeyList{"z"} // unknown actions survive for the warning
	want.Structure = Structure{
		Ignore: []string{"/etc/gone.conf", `C:\odd "path"`},
		Categories: []*Folder{
			{Catalog: "dns", Name: "DNS", Entries: []*Entry{
				{Name: "hosts", Desc: "Static \"hosts\"\tand\nmore", Path: "/etc/hosts", Kind: kind.File, Hint: "none"},
				{Name: "resolved.conf.d", Path: "/etc/systemd/resolved.conf.d", Kind: kind.Dir},
			}, Folders: []*Folder{
				{Name: "VPN", Entries: []*Entry{{Name: "wg0", Path: "/etc/wireguard/*.conf", Kind: kind.Glob}},
					Folders: []*Folder{{Name: "Deep", Scanned: true, Entries: []*Entry{
						{Name: "x", Path: "~/x.conf", Kind: kind.File, Scanned: true, Defaults: "/usr/lib/x.conf"}}}}},
			}},
			{Name: "My Configs"},
		},
		Entries: []*Entry{{Name: "top", Path: "/etc/top.conf", Kind: kind.File}},
	}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, info, err := Load(path)
	if err != nil || !info.Exists || !info.HasStructure {
		t.Fatalf("Load() info=%+v err=%v\n%s", info, err, read(t, path))
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v\n%s", got, want, read(t, path))
	}
}

func TestSavedFileIsCommented(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg := Default()
	cfg.Structure.Categories = []*Folder{{Catalog: "dns", Name: "DNS"}}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	data := read(t, path)
	for _, want := range []string{
		"# Command that opens files",
		"# Open confnav's settings (config.toml).\nsettings = [\"S\"]\n",
		"# Move up.\nup = [\"up\", \"k\"]\n",
		"[category_structure]\n",
		"# ---- DNS ----\n[[category_structure.category]]\ncatalog = \"dns\"\n",
	} {
		if !strings.Contains(data, want) {
			t.Errorf("config lacks %q:\n%s", want, data)
		}
	}
	// Every setting and key line outside the structure has its comment right above it.
	lines := strings.Split(data[:strings.Index(data, "[category_structure]")], "\n")
	for i, l := range lines {
		if strings.Contains(l, " = ") && !strings.HasPrefix(l, "#") && !strings.HasPrefix(lines[i-1], "#") {
			t.Errorf("line %q has no comment above it", l)
		}
	}
}

func TestEmptyStructureIsStillDefined(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(path, Default()); err != nil {
		t.Fatal(err)
	}
	if _, info, err := Load(path); err != nil || !info.HasStructure {
		t.Errorf("an emptied structure must not look like an old config: %+v %v", info, err)
	}
	os.WriteFile(path, []byte("editor = \"nano\"\n"), 0o644)
	if _, info, _ := Load(path); info.HasStructure {
		t.Error("a config without [category_structure] must say so")
	}
}

func TestStructureIsValidated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(path, []byte("[[category_structure.category]]\nname = \"A\"\n[[category_structure.category.entry]]\npath = \"/etc/a.conf\"\n"), 0o644)
	cfg, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if e := cfg.Structure.Categories[0].Entries[0]; e.Name != "a.conf" || e.Kind != kind.File {
		t.Errorf("defaults not filled: %+v", e)
	}
	for name, data := range map[string]string{
		"no path":  "[[category_structure.entry]]\nname = \"x\"\n",
		"bad kind": "[[category_structure.entry]]\npath = \"/x\"\nkind = \"folder\"\n",
		"no name":  "[[category_structure.category]]\ncatalog = \"x\"\n",
	} {
		os.WriteFile(path, []byte(data), 0o644)
		if _, _, err := Load(path); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestRemovedActionsAreDropped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(path, []byte("[keys]\nrestore = \"R\"\n"), 0o644)
	cfg, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Keys["restore"]; ok {
		t.Error("restore was removed and must not be reported as unknown")
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestLoadFillsInvalidValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(path, []byte("editor = \"nano\"\nrecent_limit = 0\n"), 0o644)
	got, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Editor != "nano" || got.RecentLimit != 20 || got.DateFormat == "" || !got.Mouse || !got.ShowMissing {
		t.Errorf("unexpected config %+v", got)
	}
}

func TestKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(path, []byte("[keys]\nadd = \"+\"\nedit = [\"r\", \"f2\"]\nbogus = \"x\"\n"), 0o644)
	got, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Keys["add"], KeyList{"+"}) || !reflect.DeepEqual(got.Keys["edit"], KeyList{"r", "f2"}) {
		t.Errorf("configured keys not read: add=%v edit=%v", got.Keys["add"], got.Keys["edit"])
	}
	if !reflect.DeepEqual(got.Keys["quit"], KeyList{"q"}) {
		t.Errorf("unconfigured action lost its default: %v", got.Keys["quit"])
	}
	if _, ok := got.Keys["bogus"]; !ok {
		t.Error("unknown actions must be kept so the UI can warn about them")
	}

	os.WriteFile(path, []byte("[keys]\nadd = 3\n"), 0o644)
	if _, _, err := Load(path); err == nil {
		t.Error("a number as key must be an error")
	}
}

func TestLoadBroken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(path, []byte("editor = \n"), 0o644)
	if _, info, err := Load(path); err == nil || !info.Exists {
		t.Errorf("expected parse error, got info=%+v err=%v", info, err)
	}
}
