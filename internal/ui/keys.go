package ui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/fath-kip/confnav/internal/config"
)

// keymap maps the key strings Bubble Tea reports to action names from
// config.DefaultKeys, and back.
type keymap struct {
	byKey map[string]string
	keys  map[string][]string
}

// buildKeymap combines the configured keys with the defaults. Problems are
// returned as warnings instead of errors so a typo never locks the user out.
func buildKeymap(cfg map[string]config.KeyList) (keymap, []string) {
	km := keymap{byKey: map[string]string{}, keys: map[string][]string{}}
	var warns []string

	known := map[string]bool{}
	for _, b := range config.DefaultKeys {
		known[b.Action] = true
	}
	var unknown []string
	for a := range cfg {
		if !known[a] {
			unknown = append(unknown, a)
		}
	}
	sort.Strings(unknown)
	for _, a := range unknown {
		warns = append(warns, fmt.Sprintf("unknown action %q", a))
	}

	for _, b := range config.DefaultKeys {
		list, ok := cfg[b.Action]
		if !ok {
			list = b.Keys
		}
		for _, k := range list {
			k = normalizeKey(k)
			switch other, taken := km.byKey[k]; {
			case k == "":
			case k == "ctrl+c":
				warns = append(warns, "ctrl+c is reserved for quitting")
			case taken:
				warns = append(warns, fmt.Sprintf("%s is bound to both %s and %s; %s wins", keyLabel(k), other, b.Action, other))
			default:
				km.byKey[k] = b.Action
				km.keys[b.Action] = append(km.keys[b.Action], k)
			}
		}
	}
	return km, warns
}

// normalizeKey turns what people write into what Bubble Tea reports:
// "space" is " " and "shift+e" is "E".
func normalizeKey(k string) string {
	k = strings.TrimSpace(k)
	switch {
	case strings.EqualFold(k, "space"):
		return " "
	case len(k) == 7 && strings.HasPrefix(strings.ToLower(k), "shift+") && unicode.IsLetter(rune(k[6])):
		return strings.ToUpper(k[6:])
	}
	return k
}

// keyLabel is how a key is shown on screen: "E" becomes "shift+e".
func keyLabel(k string) string {
	switch k {
	case " ":
		return "space"
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "left":
		return "←"
	case "right":
		return "→"
	}
	if r := []rune(k); len(r) == 1 && unicode.IsUpper(r[0]) {
		return "shift+" + string(unicode.ToLower(r[0]))
	}
	return k
}

// label shows the first key of an action, or "" when it has none.
func (km keymap) label(action string) string {
	if ks := km.keys[action]; len(ks) > 0 {
		return keyLabel(ks[0])
	}
	return ""
}

// labels shows every key of an action, e.g. "↑/k".
func (km keymap) labels(action string) string {
	var out []string
	for _, k := range km.keys[action] {
		out = append(out, keyLabel(k))
	}
	return strings.Join(out, "/")
}
