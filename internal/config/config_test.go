package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadMissing(t *testing.T) {
	cfg, exists, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil || exists {
		t.Fatalf("Load() exists=%v err=%v", exists, err)
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
	want.Keys["edit"] = KeyList{"r", "f2"}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, exists, err := Load(path)
	if err != nil || !exists {
		t.Fatalf("Load() exists=%v err=%v", exists, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
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
	if _, exists, err := Load(path); err == nil || !exists {
		t.Errorf("expected parse error, got exists=%v err=%v", exists, err)
	}
}
