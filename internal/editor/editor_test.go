package editor

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fake replaces the system lookups for one test.
func fake(t *testing.T, installed []string, uid int, canWrite bool) {
	t.Helper()
	oldLook, oldUID, oldW := lookPath, geteuid, writable
	t.Cleanup(func() { lookPath, geteuid, writable = oldLook, oldUID, oldW })
	lookPath = func(name string) (string, error) {
		if slices.Contains(installed, name) {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
	geteuid = func() int { return uid }
	writable = func(string) bool { return canWrite }
}

func TestResolve(t *testing.T) {
	fake(t, []string{"nano", "vim"}, 1000, true)
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	if got := Resolve(""); got != "nano" {
		t.Errorf("Resolve() with nothing set = %q, want first detected", got)
	}
	t.Setenv("EDITOR", "vim")
	if got := Resolve(""); got != "vim" {
		t.Errorf("Resolve() = %q, want $EDITOR", got)
	}
	t.Setenv("VISUAL", "nvim")
	if got := Resolve(""); got != "nvim" {
		t.Errorf("Resolve() = %q, want $VISUAL", got)
	}
	if got := Resolve(" micro "); got != "micro" {
		t.Errorf("Resolve(micro) = %q, want configured", got)
	}
	fake(t, nil, 1000, true)
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	if got := Resolve(""); got != "vi" {
		t.Errorf("Resolve() fallback = %q", got)
	}
}

func TestDetect(t *testing.T) {
	fake(t, []string{"vim", "emacs", "micro"}, 1000, true)
	if got := Detect(); !slices.Equal(got, []string{"micro", "vim", "emacs -nw"}) {
		t.Errorf("Detect() = %v", got)
	}
}

func TestPrepareDirect(t *testing.T) {
	fake(t, []string{"sudo", "sudoedit"}, 1000, true)
	l, err := Prepare("nvim -p", "/home/u/.bashrc")
	if err != nil {
		t.Fatal(err)
	}
	if l.Mode != Direct || !slices.Equal(l.Cmd.Args, []string{"nvim", "-p", "/home/u/.bashrc"}) {
		t.Errorf("got mode %v args %v", l.Mode, l.Cmd.Args)
	}
}

func TestPrepareSudoedit(t *testing.T) {
	fake(t, []string{"sudo", "sudoedit"}, 1000, false)
	dir := t.TempDir()
	real := filepath.Join(dir, "stub-resolv.conf")
	os.WriteFile(real, nil, 0o644)
	link := filepath.Join(dir, "resolv.conf")
	os.Symlink(real, link)

	l, err := Prepare("micro", link)
	if err != nil {
		t.Fatal(err)
	}
	if l.Mode != Sudoedit || !slices.Equal(l.Cmd.Args, []string{"sudoedit", real}) {
		t.Errorf("got mode %v args %v", l.Mode, l.Cmd.Args)
	}
	if !slices.Contains(l.Cmd.Env, "SUDO_EDITOR=micro") {
		t.Error("SUDO_EDITOR not set")
	}

	fake(t, nil, 1000, false)
	if _, err := Prepare("micro", "/etc/hosts"); err == nil {
		t.Error("expected an error without sudo")
	}
}

func TestPrepareRootEditsDirectly(t *testing.T) {
	fake(t, nil, 0, false)
	l, err := Prepare("nano", "/etc/fstab")
	if err != nil || l.Mode != Direct {
		t.Errorf("root: mode %v err %v", l.Mode, err)
	}
}

func TestPrepareSpecial(t *testing.T) {
	fake(t, []string{"sudo"}, 1000, true)
	tests := map[string]string{
		"/etc/sudoers":         "sudo env SUDO_EDITOR=micro VISUAL=micro EDITOR=micro visudo -f /etc/sudoers",
		"/etc/sudoers.d/wheel": "sudo env SUDO_EDITOR=micro VISUAL=micro EDITOR=micro visudo -f /etc/sudoers.d/wheel",
		"/etc/passwd":          "sudo env SUDO_EDITOR=micro VISUAL=micro EDITOR=micro vipw",
		"/etc/gshadow":         "sudo env SUDO_EDITOR=micro VISUAL=micro EDITOR=micro vigr -s",
	}
	for path, want := range tests {
		l, err := Prepare("micro", path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if got := strings.Join(l.Cmd.Args, " "); l.Mode != Special || got != want {
			t.Errorf("%s: got %q, want %q", path, got, want)
		}
	}

	fake(t, nil, 0, true)
	l, _ := Prepare("micro", "/etc/sudoers")
	if got := strings.Join(l.Cmd.Args, " "); got != "visudo -f /etc/sudoers" || !slices.Contains(l.Cmd.Env, "EDITOR=micro") {
		t.Errorf("root visudo = %q", got)
	}
}
