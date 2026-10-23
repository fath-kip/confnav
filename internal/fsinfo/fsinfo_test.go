package fsinfo

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestExpand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("CONFNAV_TEST", "/opt/x")
	tests := map[string]string{
		"~":                          home,
		"~/.ssh/config":              home + "/.ssh/config",
		"$XDG_CONFIG_HOME/pipewire":  home + "/.config/pipewire",
		"${HOME}/.bashrc":            home + "/.bashrc",
		"$CONFNAV_TEST/a.conf":       "/opt/x/a.conf",
		"/etc/hosts":                 "/etc/hosts",
		"/etc/systemd/system/*.conf": "/etc/systemd/system/*.conf",
	}
	for in, want := range tests {
		if got := Expand(in); got != want {
			t.Errorf("Expand(%q) = %q, want %q", in, got, want)
		}
	}
	t.Setenv("XDG_CONFIG_HOME", "/cfg")
	if got := Expand("$XDG_CONFIG_HOME/a"); got != "/cfg/a" {
		t.Errorf("Expand with XDG_CONFIG_HOME set = %q", got)
	}
}

func TestAbbrev(t *testing.T) {
	t.Setenv("HOME", "/home/u")
	for in, want := range map[string]string{
		"/home/u":         "~",
		"/home/u/.bashrc": "~/.bashrc",
		"/home/user2/x":   "/home/user2/x",
		"/etc/hosts":      "/etc/hosts",
	} {
		if got := Abbrev(in); got != want {
			t.Errorf("Abbrev(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStatAndWritable(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.conf")
	os.WriteFile(file, []byte("x"), 0o644)

	if s := Stat(file); s.State != Present || s.IsDir || s.ModTime.IsZero() {
		t.Errorf("Stat(file) = %+v", s)
	}
	if s := Stat(dir); s.State != Present || !s.IsDir {
		t.Errorf("Stat(dir) = %+v", s)
	}
	if s := Stat(filepath.Join(dir, "missing")); s.State != Missing {
		t.Errorf("Stat(missing) = %+v", s)
	}
	if !Writable(file) || !Writable(filepath.Join(dir, "new.conf")) {
		t.Error("files in a temp dir should be writable")
	}

	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply to root")
	}
	locked := filepath.Join(dir, "locked")
	os.Mkdir(locked, 0o755)
	inner := filepath.Join(locked, "inner.conf")
	os.WriteFile(inner, []byte("x"), 0o644)
	os.Chmod(inner, 0o444)
	if Writable(inner) {
		t.Error("read-only file reported writable")
	}
	os.Chmod(locked, 0o000)
	defer os.Chmod(locked, 0o755)
	if s := Stat(inner); s.State != NoAccess {
		t.Errorf("Stat(inner of locked dir) = %+v, want NoAccess", s)
	}
	if Writable(filepath.Join(locked, "new.conf")) {
		t.Error("file in locked dir reported writable")
	}
}

func TestListDirAndGlob(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"b.service", "a.service", "c.timer"} {
		os.WriteFile(filepath.Join(dir, f), nil, 0o644)
	}
	os.Mkdir(filepath.Join(dir, "z.d"), 0o755)
	os.Symlink(filepath.Join(dir, "z.d"), filepath.Join(dir, "link.d"))

	got, err := ListDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []Child{
		{dir + "/link.d", true}, {dir + "/z.d", true},
		{dir + "/a.service", false}, {dir + "/b.service", false}, {dir + "/c.timer", false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListDir() = %v\nwant %v", got, want)
	}

	got, err = Glob(filepath.Join(dir, "*.service"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Path != dir+"/a.service" {
		t.Errorf("Glob() = %v", got)
	}
	if got, _ := Glob(filepath.Join(dir, "*.nothing")); len(got) != 0 {
		t.Errorf("empty Glob() = %v", got)
	}
}

func TestComplete(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "systemd"), 0o755)
	os.WriteFile(filepath.Join(dir, "sysctl.conf"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "hosts"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, ".hidden"), nil, 0o644)

	tests := []struct {
		in, want string
		cands    []string
	}{
		{dir + "/ho", dir + "/hosts", nil},
		{dir + "/sy", dir + "/sys", []string{"sysctl.conf", "systemd/"}},
		{dir + "/systemd", dir + "/systemd/", nil},
		{dir + "/nothing", dir + "/nothing", nil},
		{dir + "/.h", dir + "/.hidden", nil},
		{"", "/", nil},
		{"~", "~/", nil},
	}
	for _, tt := range tests {
		got, cands := Complete(tt.in)
		if got != tt.want || !reflect.DeepEqual(cands, tt.cands) {
			t.Errorf("Complete(%q) = %q %v, want %q %v", tt.in, got, cands, tt.want, tt.cands)
		}
	}

	t.Setenv("HOME", dir)
	if got, _ := Complete("~/ho"); got != "~/hosts" {
		t.Errorf("Complete(~/ho) = %q, want ~/hosts", got)
	}
}

func TestBackup(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "etc", "hosts")
	os.MkdirAll(filepath.Dir(src), 0o755)
	os.WriteFile(src, []byte("127.0.0.1 localhost\n"), 0o644)

	now := time.Date(2026, 10, 23, 14, 5, 9, 0, time.UTC)
	dst, err := Backup(src, filepath.Join(dir, "backups"), now)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dst)[:16] != "20261023-140509_" {
		t.Errorf("unexpected backup name %q", dst)
	}
	if data, _ := os.ReadFile(dst); string(data) != "127.0.0.1 localhost\n" {
		t.Errorf("backup content = %q", data)
	}
}

func TestCopyFileNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src"), filepath.Join(dir, "dst")
	os.WriteFile(src, []byte("new"), 0o644)
	os.WriteFile(dst, []byte("old"), 0o644)
	if err := CopyFile(src, dst, 0o644); err == nil {
		t.Error("CopyFile overwrote an existing file")
	}
	if data, _ := os.ReadFile(dst); string(data) != "old" {
		t.Errorf("dst = %q", data)
	}
}
