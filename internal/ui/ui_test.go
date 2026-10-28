package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/fath-kip/confnav/internal/catalog"
	"github.com/fath-kip/confnav/internal/config"
	"github.com/fath-kip/confnav/internal/distro"
	"github.com/fath-kip/confnav/internal/fsinfo"
	"github.com/fath-kip/confnav/internal/kind"
	"github.com/fath-kip/confnav/internal/recent"
	"github.com/fath-kip/confnav/internal/scan"
	"github.com/fath-kip/confnav/internal/tree"
)

type env struct {
	m    *Model
	etc  string // fake /etc with real files
	home string
}

// setup builds a model over a small catalog that points into a temp dir.
func setup(t *testing.T, firstRun bool) *env {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	etc := filepath.Join(root, "etc")
	vendor := filepath.Join(root, "usr")
	for _, d := range []string{home, etc, vendor, filepath.Join(etc, "sysctl.d")} {
		os.MkdirAll(d, 0o755)
	}
	os.WriteFile(filepath.Join(etc, "hosts"), []byte("127.0.0.1 localhost\n"), 0o644)
	os.WriteFile(filepath.Join(etc, "sysctl.d", "99-swappiness.conf"), nil, 0o644)
	os.WriteFile(filepath.Join(vendor, "journald.conf"), []byte("[Journal]\n#Storage=auto\n"), 0o644)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local/share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local/state"))
	t.Setenv("TMUX", "")

	cat, err := catalog.Parse([]byte(fmt.Sprintf(`
[[category]]
id = "dns"
name = "DNS"
  [[category.entry]]
  name = "hosts"
  desc = "Static hostname to IP mappings"
  path = "%[1]s/hosts"
  hint = "nothing to reload"
  [[category.entry]]
  name = "resolv.conf"
  desc = "Nameservers"
  path = "%[1]s/resolv.conf"

[[category]]
id = "kernel"
name = "Kernel"
  [[category.entry]]
  name = "sysctl.d"
  desc = "Kernel parameters"
  path = "%[1]s/sysctl.d"
  kind = "dir"
  hint = "sudo sysctl --system"

[[category]]
id = "systemd"
name = "Systemd"
  [[category.entry]]
  name = "journald.conf"
  desc = "Journal settings"
  path = "%[1]s/journald.conf"
  defaults = "%[2]s/journald.conf"
`, etc, vendor)))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Editor = "true" // a command that exists and does nothing
	if !firstRun {
		cfg.Structure = cat.DefaultStructure(distro.Arch)
	}
	m := New(Options{
		Catalog: cat, Distro: distro.Info{Name: "Test Linux", Family: distro.Arch}, Family: distro.Arch,
		Config: cfg, ConfigPath: config.Path(), FirstRun: firstRun,
		Recent: recent.Load(recent.Path(), 20), Version: "1.2.3",
	})
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	return &env{m: m, etc: etc, home: home}
}

var special = map[string]tea.KeyType{
	"enter": tea.KeyEnter, "esc": tea.KeyEsc, "down": tea.KeyDown, "up": tea.KeyUp,
	"tab": tea.KeyTab, "backspace": tea.KeyBackspace, "left": tea.KeyLeft,
}

// press sends keys; multi-letter names are special keys, everything else is typed.
func (e *env) press(keys ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		var msg tea.KeyMsg
		if t, ok := special[k]; ok {
			msg = tea.KeyMsg{Type: t}
		} else {
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		_, cmd = e.m.Update(msg)
	}
	return cmd
}

func (e *env) screen() string { return ansi.Strip(e.m.View()) }

// goTo moves the cursor to the first row whose node has the given name.
func (e *env) goTo(t *testing.T, name string) {
	t.Helper()
	for i, r := range e.m.rows {
		if r.msg == "" && r.node != nil && r.node.Name == name {
			e.m.cursor = i
			return
		}
	}
	t.Fatalf("no row named %q in:\n%s", name, e.screen())
}

func (e *env) has(t *testing.T, want ...string) {
	t.Helper()
	s := e.screen()
	for _, w := range want {
		if !strings.Contains(s, w) {
			t.Errorf("screen lacks %q:\n%s", w, s)
		}
	}
}

func (e *env) lacks(t *testing.T, unwanted ...string) {
	t.Helper()
	s := e.screen()
	for _, w := range unwanted {
		if strings.Contains(s, w) {
			t.Errorf("screen should not contain %q:\n%s", w, s)
		}
	}
}

func TestInitialScreen(t *testing.T) {
	e := setup(t, false)
	e.has(t, "confnav", "Test Linux · arch", "NAME", "DESCRIPTION", "PATH", "MODIFIED",
		"▸ Recent (0)", "▸ ★ My Configs (0)", "▸ DNS (2)", "▸ Kernel (1)")
	e.lacks(t, "hosts")
	if lines := strings.Count(e.m.View(), "\n") + 1; lines != 30 {
		t.Errorf("view has %d lines, want exactly the terminal height 30", lines)
	}
}

func TestExpandShowsColumns(t *testing.T) {
	e := setup(t, false)
	e.goTo(t, "DNS")
	e.press("enter")
	e.has(t, "▾ DNS (2)", "    hosts", "Static hostname to IP mappings", e.etc+"/hosts", "missing")
	e.goTo(t, "hosts")
	e.has(t, "after edit: nothing to reload")
	e.press("left") // jumps to the parent
	if e.m.current().Name != "DNS" {
		t.Errorf("left should go to the parent, cursor on %q", e.m.current().Name)
	}
	e.press("left")
	e.lacks(t, "    hosts")
}

func TestDirectoryExpandsLazily(t *testing.T) {
	e := setup(t, false)
	e.goTo(t, "Kernel")
	e.press("enter")
	e.has(t, "▸ sysctl.d/")
	e.goTo(t, "sysctl.d")
	e.press("enter")
	e.has(t, "▾ sysctl.d/", "      99-swappiness.conf", "after edit: sudo sysctl --system")

	// A dynamic child is a real file: removing it must be refused.
	e.goTo(t, "99-swappiness.conf")
	e.press("d")
	e.has(t, "confnav never deletes files")
	if _, err := os.Stat(filepath.Join(e.etc, "sysctl.d", "99-swappiness.conf")); err != nil {
		t.Fatal("file on disk was touched")
	}
}

func TestNarrowTerminalDropsDescription(t *testing.T) {
	e := setup(t, false)
	e.m.Update(tea.WindowSizeMsg{Width: 64, Height: 20})
	e.goTo(t, "DNS")
	e.press("enter")
	e.lacks(t, "DESCRIPTION")
	e.has(t, "PATH", "hosts")
	for _, l := range strings.Split(e.screen(), "\n") {
		if width(l) > 64 {
			t.Errorf("line wider than terminal: %q", l)
		}
	}
}

func TestSearchAndReveal(t *testing.T) {
	e := setup(t, false)
	e.press("/", "h", "o", "s", "t")
	if e.m.mode != modeSearch || len(e.m.rows) == 0 || e.m.rows[0].node.Name != "hosts" {
		t.Fatalf("search did not find hosts:\n%s", e.screen())
	}
	e.has(t, "/ host", "found")
	e.press("esc")
	if e.m.mode != modeNormal || e.m.current().Name != "hosts" || !e.m.expanded["cat:dns"] {
		t.Errorf("esc should reveal hosts in the tree:\n%s", e.screen())
	}
}

func TestNewFolderAndAddEntry(t *testing.T) {
	e := setup(t, false)
	e.goTo(t, "My Configs")
	e.press("a")
	e.has(t, "Add to My Configs", "What do you want to add?", "File, directory or glob", "Folder")
	e.press("down", "enter", "W", "o", "r", "k", "enter")
	mine := e.m.cfg.Structure.Categories[0]
	if len(mine.Folders) != 1 || mine.Folders[0].Name != "Work" {
		t.Fatalf("folder not created: %+v", mine.Folders)
	}
	if cfg, _, _ := config.Load(config.Path()); len(cfg.Structure.Categories[0].Folders) != 1 {
		t.Fatal("the new folder was not saved to config.toml")
	}

	// Add to the new folder, using Tab completion for the path.
	e.press("a", "enter")
	e.press(strings.Split(e.etc+"/ho", "")...)
	e.press("tab")
	e.has(t, e.etc+"/hosts")
	e.press("enter", "enter", "M", "i", "n", "e", "enter")
	work := mine.Folders[0]
	if len(work.Entries) != 1 || work.Entries[0].Name != "hosts" || work.Entries[0].Desc != "Mine" ||
		work.Entries[0].Kind != kind.File {
		t.Fatalf("entry not added: %+v", work.Entries)
	}
	e.has(t, "Added hosts")

	// Directories are recognised as such.
	e.press("a", "enter")
	e.press(strings.Split(e.etc+"/sysctl.d/", "")...)
	e.press("enter", "enter", "enter")
	if k := work.Entries[1].Kind; k != kind.Dir || work.Entries[1].Path != e.etc+"/sysctl.d" {
		t.Errorf("directory entry = %+v", work.Entries[1])
	}
}

func TestAddValidatesPath(t *testing.T) {
	e := setup(t, false)
	e.goTo(t, "DNS")
	e.press("a", "enter", "r", "e", "l", "enter", "enter", "enter")
	if e.m.mode != modeDialog {
		t.Fatal("dialog should stay open on a relative path")
	}
	e.has(t, "must be absolute")
	e.press("esc")
	if e.m.mode != modeNormal || len(e.m.cfg.Structure.Category("dns").Entries) != 2 {
		t.Error("esc should cancel without changes")
	}
}

func TestRemoveCatalogEntry(t *testing.T) {
	e := setup(t, false)
	e.goTo(t, "DNS")
	e.press("enter")
	e.goTo(t, "hosts")
	e.press("d")
	e.has(t, "Remove hosts?", "file on disk is not touched")
	e.press("y")
	e.has(t, "▾ DNS (1)", "Removed hosts")
	if _, err := os.Stat(e.etc + "/hosts"); err != nil {
		t.Fatal("the file on disk was touched")
	}
	if len(e.m.cfg.Structure.Ignore) != 0 {
		t.Error("only entries found by a scan go to the ignore list")
	}

	// Removing a built-in category points at the preferences to get it back.
	e.goTo(t, "DNS")
	e.press("d")
	e.has(t, "Remove DNS?", "preferences (shift+p)")
}

func TestCopyBuiltinToMyConfigs(t *testing.T) {
	e := setup(t, false)
	e.goTo(t, "DNS")
	e.press("enter")
	e.goTo(t, "hosts")
	e.press("c")
	e.has(t, "Copy hosts to", "My Configs")
	e.press("enter") // My Configs is the first destination
	mine := e.m.cfg.Structure.Categories[0]
	if len(mine.Entries) != 1 || mine.Entries[0].Path != e.etc+"/hosts" || mine.Entries[0].Hint != "nothing to reload" {
		t.Fatalf("copy failed: %+v", mine.Entries)
	}
	if len(e.m.cfg.Structure.Category("dns").Entries) != 2 {
		t.Error("copying must keep the original")
	}
	// The copy keeps the catalog hint.
	e.goTo(t, "My Configs")
	if !e.m.expanded[e.m.current().ID] {
		t.Error("destination should be expanded")
	}
}

func TestOpenFileAndAfterEdit(t *testing.T) {
	e := setup(t, false)
	e.goTo(t, "DNS")
	e.press("enter")
	e.goTo(t, "hosts")
	if cmd := e.press("enter"); cmd == nil {
		t.Fatal("opening an existing file should return the editor command")
	}

	path := e.etc + "/hosts"
	before := fsinfo.Stat(path)
	os.WriteFile(path, []byte("changed\n"), 0o644)
	os.Chtimes(path, before.ModTime.Add(1e9), before.ModTime.Add(1e9))
	e.m.Update(editorDoneMsg{path: path, hint: "nothing to reload", before: before})
	e.has(t, "Saved", "next: nothing to reload", "▸ Recent (1)")
	if len(e.m.recent.Items) != 1 || e.m.recent.Items[0].Path != path {
		t.Errorf("recent = %+v", e.m.recent.Items)
	}
	if _, err := os.Stat(recent.Path()); err != nil {
		t.Error("recent list not saved")
	}
}

func TestOpenMissingFileAsksFirst(t *testing.T) {
	e := setup(t, false)
	e.goTo(t, "DNS")
	e.press("enter")
	e.goTo(t, "resolv.conf")
	if cmd := e.press("enter"); cmd != nil {
		t.Error("a missing file must not open without confirmation")
	}
	e.has(t, "Create ")
	if cmd := e.press("y"); cmd == nil {
		t.Error("confirming should launch the editor")
	}
}

func TestMissingFileStartsFromDefaults(t *testing.T) {
	e := setup(t, false)
	e.m.Update(tea.WindowSizeMsg{Width: 260, Height: 30}) // temp paths are long
	e.goTo(t, "Systemd")
	e.press("enter")
	e.goTo(t, "journald.conf")
	e.has(t, "defaults in "+filepath.Dir(e.etc)+"/usr/journald.conf")
	e.press("enter")
	e.has(t, "Start from the distribution defaults", "Start with an empty file")
	copyCmd := e.press("enter")
	if copyCmd == nil {
		t.Fatal("choosing the defaults should copy them")
	}
	_, launch := e.m.Update(copyCmd())
	if launch == nil {
		t.Fatal("after copying, the editor should open")
	}
	if data, _ := os.ReadFile(e.etc + "/journald.conf"); string(data) != "[Journal]\n#Storage=auto\n" {
		t.Errorf("copied content = %q", data)
	}
}

func TestFirstRunAsksForEditor(t *testing.T) {
	e := setup(t, true)
	if e.m.mode != modeDialog {
		t.Fatal("first run should open the editor picker")
	}
	e.has(t, "Welcome to confnav! · 1/4")
	e.press("esc")
	cfg, _, err := config.Load(config.Path())
	if err != nil {
		t.Fatal("cancelling the first-run preferences should still write the config")
	}
	if got := len(cfg.Structure.Categories); got != 4 || cfg.Structure.Categories[0].Name != "My Configs" {
		t.Errorf("cancelling should keep the defaults: %d categories", got)
	}
	e.has(t, "▸ DNS (2)")
}

func TestFirstRunPreferences(t *testing.T) {
	e := setup(t, true)
	e.press("G", "enter") // Custom command…, whatever editors this machine has
	e.press("esc")        // leaving the form keeps going with defaults on the first run
	if e.m.mode == modeDialog {
		t.Fatal("esc in the custom form should end the first-run questions")
	}

	e = setup(t, true)
	e.press("G", "enter")
	for range len("true") {
		e.press("backspace")
	}
	e.press("n", "a", "n", "o", "enter")
	e.has(t, "Welcome to confnav! · 2/4", "Which categories do you want in the list?",
		"[x] DNS (2 files)", "[x] Kernel (1 files)", "3 of 3 selected", "your own and change their files", "[category_structure]")
	e.press(" ") // untick DNS
	e.has(t, "[ ] DNS (2 files)", "2 of 3 selected")
	e.press("enter")
	e.has(t, "3/4", "Show files that don't exist on this system?", "Missing files are", "shift+m")
	e.press("down", "enter")
	e.has(t, "4/4", "Look for more configuration files", "Scan as root (sudo asks for your password)", "Don't scan now")
	e.press("G", "enter")
	if e.m.mode != modeNormal {
		t.Fatalf("the last answer should close the preferences:\n%s", e.screen())
	}
	cfg, _, err := config.Load(config.Path())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range cfg.Structure.Categories {
		names = append(names, f.Name)
	}
	if strings.Join(names, ",") != "My Configs,Kernel,Systemd" || cfg.Editor != "nano" || cfg.ShowMissing || cfg.ScanAs != "" {
		t.Errorf("saved %v editor=%q show_missing=%v scan_as=%q", names, cfg.Editor, cfg.ShowMissing, cfg.ScanAs)
	}
	e.has(t, "Preferences saved")
	e.lacks(t, "DNS")
}

// walkPrefs answers the editor question with the current editor and ticks
// nothing else, ending on the scan question.
func (e *env) prefsToCategories(t *testing.T) {
	t.Helper()
	e.press("P")
	e.has(t, "Preferences · 1/4")
	e.press("G", "enter", "enter") // custom command, keeping the current one
	e.has(t, "Preferences · 2/4")
}

func TestPreferencesRemoveAndAddCategories(t *testing.T) {
	e := setup(t, false)
	e.prefsToCategories(t)
	e.press(" ", "enter", "enter", "enter") // untick DNS; keep missing files; don't scan
	e.has(t, "Remove the category DNS?", "2 entries")
	e.press("n")
	if e.m.cfg.Structure.Category("dns") == nil {
		t.Fatal("n must keep the category")
	}
	e.has(t, "Preferences saved")

	e.prefsToCategories(t)
	e.press(" ", "enter", "enter", "enter", "y")
	if e.m.cfg.Structure.Category("dns") != nil {
		t.Fatal("y must remove the category")
	}
	e.lacks(t, "DNS (")

	e.prefsToCategories(t)
	e.has(t, "[ ] DNS")
	e.press(" ", "enter", "enter", "enter")
	if dns := e.m.cfg.Structure.Category("dns"); dns == nil || len(dns.Entries) != 2 {
		t.Fatal("ticking a category again should add it from the catalog")
	}

	// esc later on changes nothing.
	e.prefsToCategories(t)
	e.press(" ", "esc")
	if e.m.cfg.Structure.Category("dns") == nil || e.m.mode != modeNormal {
		t.Error("esc must cancel the preferences without changes")
	}
	e.has(t, "Preferences unchanged")
}

func TestScanAddsFoundFiles(t *testing.T) {
	e := setup(t, false)
	e.m.runScan = func(r scan.Roots) []scan.Found {
		return []scan.Found{
			{Path: e.etc + "/hosts", Category: "dns"}, // already listed
			{Path: e.etc + "/host.conf", Category: "dns"},
			{Path: e.etc + "/foo/foo.conf", Category: scan.Other, Folder: "foo", Group: e.etc + "/foo"},
			{Path: e.etc + "/pacman.conf", Category: "packages"}, // not in the list
		}
	}
	e.press("F")
	e.has(t, "How should confnav scan?", "Scan as root")
	cmd := e.press("enter")
	if e.m.cfg.ScanAs != config.ScanUser || cmd == nil {
		t.Fatalf("scan_as = %q, cmd = %v", e.m.cfg.ScanAs, cmd)
	}
	e.has(t, "scanning…")
	if again := e.press("F"); again != nil {
		t.Error("a second scan must not start while one runs")
	}
	e.m.Update(cmd())
	e.has(t, "Scan added 2 file(s) (1 already listed, 1 skipped: their category is not in your list)", "▸ Other (1)")
	e.lacks(t, "scanning…")
	dns := e.m.cfg.Structure.Category("dns")
	if len(dns.Entries) != 3 || !dns.Entries[2].Scanned || dns.Entries[2].Name != "host.conf" {
		t.Fatalf("DNS = %+v", dns.Entries)
	}

	// A removed scan result stays away.
	e.goTo(t, "DNS")
	e.press("enter")
	e.goTo(t, "host.conf")
	e.has(t, "found by scan")
	e.press("d")
	e.has(t, "A new scan won't add it again")
	e.press("y")
	e.m.Update(e.press("F")())
	e.has(t, "Scan added 0 file(s) (3 already listed, 1 skipped")
}

func TestRemoveUserFolder(t *testing.T) {
	e := setup(t, false)
	e.goTo(t, "My Configs")
	e.press("d")
	e.has(t, "Remove My Configs?", "file on disk is not touched")
	e.press("n")
	if e.m.cfg.Structure.Categories[0].Name != "My Configs" {
		t.Fatal("n must keep the folder")
	}
	e.press("d", "y")
	if e.m.cfg.Structure.Categories[0].Name == "My Configs" {
		t.Error("folder not removed")
	}
	e.lacks(t, "★ My Configs")
}

func TestMouseTogglesFolder(t *testing.T) {
	e := setup(t, false)
	e.goTo(t, "Recent")
	var dnsRow int
	for i, r := range e.m.rows {
		if r.node != nil && r.node.ID == "cat:dns" {
			dnsRow = i
		}
	}
	e.m.Update(tea.MouseMsg{X: 3, Y: listTop + dnsRow, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if !e.m.expanded["cat:dns"] {
		t.Errorf("click should expand DNS:\n%s", e.screen())
	}
}

func TestBatchedRunesAreSeparateKeys(t *testing.T) {
	e := setup(t, false)
	e.m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jj")})
	if e.m.current().ID != "cat:dns" {
		t.Errorf("\"jj\" should move down twice, cursor on %q", e.m.current().Name)
	}
	e.m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/hosts")})
	if e.m.mode != modeSearch || e.m.search.Value() != "hosts" {
		t.Errorf("\"/hosts\" should start a search for hosts, got mode %v value %q", e.m.mode, e.m.search.Value())
	}
}

func TestHelp(t *testing.T) {
	e := setup(t, false)
	e.press("?")
	e.has(t, "Help · confnav 1.2.3", "╭", "Move", "Organise", "sudoedit", "esc close help")
	e.lacks(t, "NAME", "Files you opened recently")
	e.press("G")
	e.has(t, "never deletes", "Mouse", "double-click")
	e.press("x")
	if e.m.mode != modeNormal {
		t.Error("a key that does not scroll should close help")
	}
}

func TestHelpScrolls(t *testing.T) {
	e := setup(t, false)
	e.m.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	e.press("?")
	e.has(t, "↓ more below", "Move")
	e.press("G")
	e.lacks(t, "↓ more below")
	e.has(t, "never deletes")
	for _, l := range strings.Split(e.screen(), "\n") {
		if width(l) > 80 {
			t.Errorf("help line wider than the terminal: %q", l)
		}
	}
	if cmd := e.press("q"); cmd != nil || e.m.mode != modeNormal {
		t.Error("q should close help, not quit")
	}
}

// TestRender logs a full screen; run with -v to eyeball the layout.
func TestRender(t *testing.T) {
	e := setup(t, false)
	e.goTo(t, "DNS")
	e.press("enter")
	e.goTo(t, "Kernel")
	e.press("enter", "down", "enter")
	e.goTo(t, "hosts")
	t.Log("\n" + e.screen())
	_ = tree.RecentID
}

func TestFirstRunModalIsCentredAndProminent(t *testing.T) {
	e := setup(t, true)
	lines := strings.Split(e.screen(), "\n")
	title := -1
	for i, l := range lines {
		if strings.Contains(l, "Welcome to confnav!") {
			title = i
		}
	}
	if title < listTop+3 || title > len(lines)/2 {
		t.Errorf("welcome title on line %d of %d, want it near the middle:\n%s", title, len(lines), e.screen())
	}
	if lead := len(lines[title]) - len(strings.TrimLeft(lines[title], " ")); lead < 10 {
		t.Errorf("modal is not centred (indent %d):\n%s", lead, e.screen())
	}
	e.has(t, "Which editor should open your files?", "shift+e", "› ", "choose", "esc cancel")
	e.lacks(t, "▸ DNS") // the list makes room for the modal
}

func TestKeysLineUsesShiftNames(t *testing.T) {
	e := setup(t, false)
	e.has(t, "shift+s settings", "shift+p prefs", "shift+m hide missing", "a add")
	e.lacks(t, " n folder")
	e.press("?")
	e.has(t, "shift+f    scan the system", "↑/k", "move up", "config.toml")
}

func TestPlusRowAddsAtTopLevel(t *testing.T) {
	e := setup(t, false)
	e.press("G")
	if !e.m.onPlus() {
		t.Fatalf("the last row should be +:\n%s", e.screen())
	}
	e.has(t, "Add a folder or file at the top level")
	e.press("enter")
	e.has(t, "Add to the top level")
	e.press("enter")
	e.press(strings.Split(e.etc+"/hosts", "")...)
	e.press("enter", "enter", "enter")
	top := e.m.cfg.Structure.Entries
	if len(top) != 1 || top[0].Kind != kind.File || top[0].Path != e.etc+"/hosts" {
		t.Fatalf("top-level file not added: %+v", top)
	}
	e.has(t, "\n  hosts ")

	// A top-level file adds next to itself, at the top level.
	e.goTo(t, "hosts")
	e.press("a")
	e.has(t, "Add to the top level")
}

func TestAddFolderInsideCategory(t *testing.T) {
	e := setup(t, false)
	e.goTo(t, "DNS")
	e.press("a", "down", "enter", "V", "P", "N", "enter")
	dns := e.m.cfg.Structure.Category("dns")
	if len(dns.Folders) != 1 || dns.Folders[0].Name != "VPN" {
		t.Fatalf("folder not added to DNS: %+v", dns.Folders)
	}
	e.has(t, "▾ DNS (3)", "  ▸ ★ VPN (0)")
}

func TestDoubleClickOpensFile(t *testing.T) {
	e := setup(t, false)
	e.goTo(t, "DNS")
	e.press("enter")
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	e.m.now = func() time.Time { return clock }
	var row int
	for i, r := range e.m.rows {
		if r.node != nil && r.node.Name == "hosts" {
			row = i
		}
	}
	click := func() tea.Cmd {
		_, cmd := e.m.Update(tea.MouseMsg{X: 5, Y: listTop + row, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		return cmd
	}
	if click() != nil {
		t.Error("a single click must not open a file")
	}
	clock = clock.Add(2 * time.Second)
	if click() != nil {
		t.Error("two slow clicks are not a double click")
	}
	clock = clock.Add(200 * time.Millisecond)
	if click() == nil {
		t.Error("a double click should open the file")
	}
}

func TestToggleMissing(t *testing.T) {
	e := setup(t, false)
	e.goTo(t, "DNS")
	e.press("enter")
	e.press("M")
	if e.m.cfg.ShowMissing || e.m.missingHidden != 2 {
		t.Fatalf("show_missing=%v hidden=%d", e.m.cfg.ShowMissing, e.m.missingHidden)
	}
	e.has(t, "2 missing hidden", "▾ DNS (1)", "shift+m show missing")
	e.lacks(t, "resolv.conf", "Systemd") // Systemd only had a missing file
	cfg, _, _ := config.Load(config.Path())
	if cfg.ShowMissing {
		t.Error("the choice was not saved")
	}
	e.press("M")
	e.has(t, "resolv.conf", "▸ Systemd (1)")
	e.lacks(t, "missing hidden")
}

func TestSettingsKeyAndReload(t *testing.T) {
	e := setup(t, false)
	e.lacks(t, "confnav settings") // the settings key replaced the top-level entry
	e.has(t, "shift+s settings")
	if cmd := e.press("S"); cmd == nil {
		t.Fatal("opening the settings should launch the editor right away")
	}
	before := fsinfo.Stat(config.Path())
	if before.State != fsinfo.Present {
		t.Fatal("settings file was not created with defaults")
	}

	// The user binds add to "+" and saves.
	edited := strings.Replace(readFile(t, config.Path()), "add = [\"a\"]", "add = \"+\"", 1)
	writeLater(t, config.Path(), edited, before)
	e.m.Update(editorDoneMsg{path: config.Path(), before: before})
	e.has(t, "Settings reloaded")
	if e.m.keys.byKey["+"] != "add" || e.m.keys.byKey["a"] != "" {
		t.Fatalf("new key not applied: %v", e.m.keys.byKey["+"])
	}
	e.has(t, "+ add")

	// A broken file keeps the current settings.
	before = fsinfo.Stat(config.Path())
	writeLater(t, config.Path(), "editor = \n", before)
	e.m.Update(editorDoneMsg{path: config.Path(), before: before})
	e.has(t, "previous settings kept")
	if e.m.keys.byKey["+"] != "add" {
		t.Error("broken config must not reset the keys")
	}
}

func TestBadKeysWarn(t *testing.T) {
	e := setup(t, false)
	e.m.cfg.Keys["edit"] = config.KeyList{"a"}
	os.MkdirAll(filepath.Dir(config.Path()), 0o755)
	config.Save(config.Path(), e.m.cfg)
	cmd, err := e.m.reloadConfig()
	if err != nil || cmd != nil {
		t.Fatal(err)
	}
	e.has(t, "a is bound to both add and edit; add wins")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// writeLater writes content with a modification time after before's.
func writeLater(t *testing.T, path, content string, before fsinfo.Info) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	later := before.ModTime.Add(time.Second)
	os.Chtimes(path, later, later)
}
