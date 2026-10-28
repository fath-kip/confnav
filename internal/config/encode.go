package config

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/fath-kip/confnav/internal/kind"
)

const header = `# confnav configuration
#
# Lines starting with # are comments. Each setting is explained right above it.
# Save this file while confnav is running (shift+s opens it) and the new
# settings apply at once; a file with an error keeps the previous settings.
#
# confnav rewrites this file when you change something in the app (adding or
# removing entries, shift+e, shift+m, preferences, scans), so comments you add
# yourself are not kept.
`

const keysHeader = `# ---------------------------------------------------------------------------
# Keys
#
# Every action with the keys that trigger it: one key ("a") or a list
# (["r", "f2"]). Upper case means shift ("E" is shift+e). Other names:
# "enter", "space", "esc", "tab", "up", "down", "left", "right", "home", "end",
# "pgup", "pgdown", "delete", "f1"…"f12", "ctrl+r". ctrl+c always quits.
`

const structureHeader = `# ---------------------------------------------------------------------------
# Categories and files
#
# Everything the list shows, in order. Edit it here or in the app (a add,
# r rename/edit, d remove, c copy, m move); shift+p adds built-in categories
# back.
#
#   [[category_structure.category]]                a top-level category
#     catalog        built-in topic it came from ("dns", "network", …);
#                    leave it out for your own categories
#     name           shown in the list
#   [[category_structure.category.entry]]          a file inside it
#     name, desc     shown in the list (name defaults to the file name)
#     path           absolute or ~/…; $XDG_CONFIG_HOME works too
#     kind           "file" (default), "dir" (expands to its contents) or
#                    "glob" (e.g. "/etc/sysctl.d/*.conf")
#     hint           reminder shown after you save, e.g. a restart command
#     defaults       vendor copy offered as a start when path doesn't exist
#     found_by_scan  added by a scan (shift+f)
#   [[category_structure.category.folder]]         a folder in a category;
#   [[category_structure.category.folder.entry]]   folders nest the same way
#   [[category_structure.entry]]                   a file at the top level
`

// settingDocs are the comments above each top-level setting.
var settingDocs = map[string]string{
	"editor": "Command that opens files, e.g. \"micro\", \"nano\", \"vim\", \"nvim -p\".\n" +
		"Empty means: $VISUAL, then $EDITOR, then the first installed editor. (shift+e)",
	"backup":       "Copy a file to ~/.local/state/confnav/backups before opening it.",
	"mouse":        "Mouse clicks and scrolling (hold Shift to select text).",
	"recent_limit": "How many files the Recent list keeps.",
	"date_format":  "Go time layout for the Modified column (\"2006-01-02 15:04\" is YYYY-MM-DD HH:MM).",
	"show_missing": "List files that don't exist on this system yet, greyed out. (shift+m)",
	"scan_as": "How a scan (shift+f) runs: \"user\" sees what you can read, \"root\" uses sudo\n" +
		"to also see protected directories. Empty: ask on the next scan.",
}

func encode(cfg Config) []byte {
	var b bytes.Buffer
	b.WriteString(header)
	setting := func(key, value string) {
		b.WriteString("\n")
		comment(&b, "", settingDocs[key])
		fmt.Fprintf(&b, "%s = %s\n", key, value)
	}
	setting("editor", quote(cfg.Editor))
	setting("backup", strconv.FormatBool(cfg.Backup))
	setting("mouse", strconv.FormatBool(cfg.Mouse))
	setting("recent_limit", strconv.Itoa(cfg.RecentLimit))
	setting("date_format", quote(cfg.DateFormat))
	setting("show_missing", strconv.FormatBool(cfg.ShowMissing))
	setting("scan_as", quote(cfg.ScanAs))

	b.WriteString("\n" + keysHeader + "[keys]\n")
	known := map[string]bool{}
	for _, kb := range DefaultKeys {
		known[kb.Action] = true
		keys, ok := cfg.Keys[kb.Action]
		if !ok {
			keys = kb.Keys
		}
		b.WriteString("\n")
		comment(&b, "", upperFirst(kb.Desc)+".")
		fmt.Fprintf(&b, "%s = %s\n", kb.Action, quoteList(keys))
	}
	var unknown []string
	for a := range cfg.Keys {
		if !known[a] {
			unknown = append(unknown, a)
		}
	}
	sort.Strings(unknown)
	for _, a := range unknown {
		b.WriteString("\n# Unknown action: confnav ignores it.\n")
		fmt.Fprintf(&b, "%s = %s\n", bareKey(a), quoteList(cfg.Keys[a]))
	}

	s := cfg.Structure
	b.WriteString("\n" + structureHeader + "[category_structure]\n\n")
	comment(&b, "", "Paths you removed after a scan; a new scan won't add them again.")
	if len(s.Ignore) == 0 {
		b.WriteString("ignore = []\n")
	} else {
		b.WriteString("ignore = [\n")
		for _, p := range s.Ignore {
			fmt.Fprintf(&b, "  %s,\n", quote(p))
		}
		b.WriteString("]\n")
	}
	for _, f := range s.Categories {
		writeFolder(&b, "category_structure.category", f, "")
	}
	for _, e := range s.Entries {
		writeEntry(&b, "category_structure.entry", e, "")
	}
	return b.Bytes()
}

func writeFolder(b *bytes.Buffer, table string, f *Folder, indent string) {
	if indent == "" {
		fmt.Fprintf(b, "\n# ---- %s ----\n", f.Name)
	} else {
		b.WriteString("\n")
	}
	fmt.Fprintf(b, "%s[[%s]]\n", indent, table)
	if f.Catalog != "" {
		fmt.Fprintf(b, "%scatalog = %s\n", indent, quote(f.Catalog))
	}
	fmt.Fprintf(b, "%sname = %s\n", indent, quote(f.Name))
	if f.Scanned {
		fmt.Fprintf(b, "%sfound_by_scan = true\n", indent)
	}
	for _, e := range f.Entries {
		writeEntry(b, table+".entry", e, indent+"  ")
	}
	for _, c := range f.Folders {
		writeFolder(b, table+".folder", c, indent+"  ")
	}
}

func writeEntry(b *bytes.Buffer, table string, e *Entry, indent string) {
	fmt.Fprintf(b, "\n%s[[%s]]\n", indent, table)
	field := func(k, v string) {
		if v != "" {
			fmt.Fprintf(b, "%s%s = %s\n", indent, k, quote(v))
		}
	}
	field("name", e.Name)
	field("desc", e.Desc)
	field("path", e.Path)
	if e.Kind != kind.File {
		field("kind", string(e.Kind))
	}
	field("hint", e.Hint)
	field("defaults", e.Defaults)
	if e.Scanned {
		fmt.Fprintf(b, "%sfound_by_scan = true\n", indent)
	}
}

func comment(b *bytes.Buffer, indent, text string) {
	for _, l := range strings.Split(text, "\n") {
		fmt.Fprintf(b, "%s# %s\n", indent, l)
	}
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func quoteList(l KeyList) string {
	parts := make([]string, len(l))
	for i, k := range l {
		parts[i] = quote(k)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// bareKey quotes a key unless TOML allows it bare.
func bareKey(k string) string {
	for _, r := range k {
		bare := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-'
		if !bare {
			return quote(k)
		}
	}
	if k == "" {
		return `""`
	}
	return k
}

// quote returns s as a TOML basic string.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\f':
			b.WriteString(`\f`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
