package ui

import (
	"strings"
	"testing"

	"github.com/fath-kip/confnav/internal/config"
)

func TestKeyLabel(t *testing.T) {
	for in, want := range map[string]string{
		"E": "shift+e", "a": "a", " ": "space", "up": "↑", "ctrl+r": "ctrl+r", "?": "?", "/": "/",
	} {
		if got := keyLabel(in); got != want {
			t.Errorf("keyLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeKey(t *testing.T) {
	for in, want := range map[string]string{
		"space": " ", "Space": " ", "shift+e": "E", "Shift+E": "E", " a ": "a", "shift+tab": "shift+tab",
	} {
		if got := normalizeKey(in); got != want {
			t.Errorf("normalizeKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildKeymap(t *testing.T) {
	km, warns := buildKeymap(config.Default().Keys)
	if len(warns) != 0 {
		t.Errorf("defaults produce warnings: %v", warns)
	}
	for _, b := range config.DefaultKeys {
		for _, k := range b.Keys {
			if got := km.byKey[normalizeKey(k)]; got != b.Action {
				t.Errorf("default key %q -> %q, want %q", k, got, b.Action)
			}
		}
	}
	if km.label("editor") != "shift+e" || km.labels("up") != "↑/k" || km.label("open") != "enter" {
		t.Errorf("labels: %q %q %q", km.label("editor"), km.labels("up"), km.label("open"))
	}

	cfg := config.Default().Keys
	cfg["edit"] = config.KeyList{"f2", "shift+r"} // shift+r is restore's key
	cfg["quit"] = config.KeyList{"ctrl+c", "x"}
	cfg["fly"] = config.KeyList{"z"}
	km, warns = buildKeymap(cfg)
	all := strings.Join(warns, "\n")
	for _, want := range []string{`unknown action "fly"`, "ctrl+c is reserved", "shift+r is bound to both edit and restore; edit wins"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing warning %q in:\n%s", want, all)
		}
	}
	if km.byKey["f2"] != "edit" || km.byKey["r"] != "" || km.byKey["x"] != "quit" || km.byKey["R"] != "edit" {
		t.Errorf("unexpected bindings: f2=%q r=%q x=%q R=%q", km.byKey["f2"], km.byKey["r"], km.byKey["x"], km.byKey["R"])
	}
}
