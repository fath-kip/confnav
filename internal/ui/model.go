// Package ui is the Bubble Tea terminal interface.
package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"github.com/fath-kip/confnav/internal/catalog"
	"github.com/fath-kip/confnav/internal/config"
	"github.com/fath-kip/confnav/internal/distro"
	"github.com/fath-kip/confnav/internal/fsinfo"
	"github.com/fath-kip/confnav/internal/kind"
	"github.com/fath-kip/confnav/internal/recent"
	"github.com/fath-kip/confnav/internal/scan"
	"github.com/fath-kip/confnav/internal/tree"
)

// Options is what main hands to the UI.
type Options struct {
	Catalog    *catalog.Catalog
	Distro     distro.Info
	Family     distro.Family
	Config     config.Config
	ConfigPath string
	FirstRun   bool // no config file yet: open the preferences
	Recent     *recent.List
	Query      string // start in search mode with this query
	Version    string // shown on the help page
}

type mode int

const (
	modeNormal mode = iota
	modeSearch
	modeDialog
	modeHelp
)

// listTop is the screen line of the first row (after title and header).
const listTop = 2

type row struct {
	node  *tree.Node
	depth int
	info  fsinfo.Info
	msg   string // placeholder text such as "(empty)"; node is the parent
	plus  bool   // the "+" row at the bottom that adds at the top level
}

// doubleClick is the longest gap between two clicks of a double click.
const doubleClick = 500 * time.Millisecond

// Model is the application state. It is used through a pointer so dialog
// callbacks can change it.
type Model struct {
	opt     Options
	cfg     config.Config
	recent  *recent.List
	roots   []*tree.Node
	entries []*tree.Node // search corpus
	keys    keymap

	// missingHidden counts built-ins left out because show_missing is off.
	missingHidden int

	expanded map[string]bool
	rows     []row
	cursor   int
	offset   int
	width    int
	height   int

	mode     mode
	prevMode mode
	dlg      dialog
	search   textinput.Model

	status      string
	statusStyle lipgloss.Style

	lastClickRow int
	lastClickAt  time.Time

	helpOffset int // first help line on screen

	scanning bool
	runScan  func(scan.Roots) []scan.Found // scan.Run; tests replace it

	now func() time.Time
}

// New creates the model.
func New(opt Options) *Model {
	m := &Model{
		opt:      opt,
		cfg:      opt.Config,
		recent:   opt.Recent,
		expanded: map[string]bool{},
		runScan:  scan.Run,
		now:      time.Now,
	}
	m.search = textinput.New()
	m.search.Prompt = "/ "
	m.search.Placeholder = "search names, descriptions and paths"
	m.search.Cursor.SetMode(cursor.CursorStatic)
	var warns []string
	m.keys, warns = buildKeymap(m.cfg.Keys)
	m.rebuild()

	if len(warns) > 0 {
		m.setStatus(sHint, "config.toml keys: "+strings.Join(warns, "; "))
	}
	if opt.Family == distro.Unknown {
		m.setStatus(sHint, "Unknown distribution: showing common files only (try --distro arch|debian|fedora)")
	}
	if opt.Query != "" {
		m.startSearch(opt.Query)
	}
	if opt.FirstRun {
		m.openPrefs(true)
	}
	return m
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case editorDoneMsg:
		return m, m.afterEdit(msg)
	case copyDoneMsg:
		return m, m.afterCopy(msg)
	case sudoDoneMsg:
		return m, m.afterSudo(msg)
	case scanDoneMsg:
		m.afterScan(msg)
	case tea.MouseMsg:
		return m, m.updateMouse(msg)
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// Fast typing or key repeat can arrive as one message ("jj");
		// handle it as separate key presses so shortcuts still work.
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && !msg.Paste {
			var cmds []tea.Cmd
			for _, r := range msg.Runes {
				_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}
		switch m.mode {
		case modeDialog:
			return m, m.dlg.update(m, msg)
		case modeHelp:
			m.updateHelp(msg)
			return m, nil
		case modeSearch:
			return m, m.updateSearch(msg)
		default:
			return m, m.updateNormal(msg)
		}
	}
	return m, nil
}

func (m *Model) updateNormal(msg tea.KeyMsg) tea.Cmd {
	m.status = ""
	n := m.current()
	plus := m.onPlus()
	switch m.keys.byKey[msg.String()] {
	case "quit":
		return tea.Quit
	case "up":
		m.move(-1)
	case "down":
		m.move(1)
	case "page_up":
		m.move(-m.listHeight())
	case "page_down":
		m.move(m.listHeight())
	case "top":
		m.setCursor(0)
	case "bottom":
		m.setCursor(len(m.rows) - 1)
	case "collapse":
		m.collapseOrParent()
	case "expand":
		m.expandOrChild()
	case "open":
		if plus {
			m.addPrompt(nil)
			return nil
		}
		return m.activate(n)
	case "open_with":
		m.openWith(n)
	case "editor":
		m.chooseEditor()
	case "settings":
		return m.openSettings()
	case "preferences":
		m.openPrefs(false)
	case "scan":
		return m.startScan()
	case "add":
		m.addPrompt(n)
	case "edit":
		m.editNode(n)
	case "copy":
		m.copyOrMove(n, false)
	case "move":
		m.copyOrMove(n, true)
	case "remove":
		m.remove(n)
	case "toggle_missing":
		m.toggleMissing()
	case "yank":
		m.yank(n)
	case "refresh":
		cmd, err := m.reloadConfig()
		m.rebuild()
		if err != nil {
			m.fail(err.Error())
		} else if m.status == "" {
			m.setStatus(sOK, "Refreshed")
		}
		return cmd
	case "search":
		m.startSearch("")
	case "help":
		m.mode, m.helpOffset = modeHelp, 0
	}
	return nil
}

func (m *Model) updateMouse(msg tea.MouseMsg) tea.Cmd {
	if m.mode == modeHelp {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.helpOffset -= 3
		case tea.MouseButtonWheelDown:
			m.helpOffset += 3
		}
		return nil
	}
	if m.mode != modeNormal && m.mode != modeSearch {
		return nil
	}
	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		m.move(-3)
	case msg.Button == tea.MouseButtonWheelDown:
		m.move(3)
	case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
		i := m.offset + msg.Y - listTop
		if msg.Y < listTop || i < 0 || i >= len(m.rows) || msg.Y >= listTop+m.listHeight() {
			return nil
		}
		m.status = ""
		now := m.now()
		double := i == m.lastClickRow && now.Sub(m.lastClickAt) < doubleClick
		m.lastClickRow, m.lastClickAt = i, now
		if double {
			m.lastClickAt = time.Time{} // a third click starts over
		}
		m.cursor = i
		// Folders toggle on a single click; files and the + row need a
		// double click, so a stray click never launches an editor.
		r := m.rows[i]
		switch {
		case r.plus:
			if double {
				m.addPrompt(nil)
			}
		case r.msg != "":
		case r.node.Expandable() && m.mode == modeNormal:
			if !double {
				return m.activate(r.node)
			}
		case double:
			return m.activate(r.node)
		}
	}
	return nil
}

// ---- tree and cursor ----

func (m *Model) source() tree.Source {
	return tree.Source{Structure: &m.cfg.Structure, Recent: m.recent, HideMissing: !m.cfg.ShowMissing}
}

// rebuild recreates the tree from the catalog, library and recent list,
// re-reading the disk, and keeps the cursor on the same node if it still exists.
func (m *Model) rebuild() {
	m.roots, m.missingHidden = tree.Build(m.source())
	m.entries = tree.Entries(m.roots)
	m.refreshRows()
}

// refreshRows recomputes the visible rows without rebuilding the tree.
func (m *Model) refreshRows() {
	id := ""
	if n := m.current(); n != nil {
		id = n.ID
	}
	if m.mode == modeSearch || (m.mode == modeDialog && m.prevMode == modeSearch) {
		m.runSearch()
	} else {
		m.flatten()
	}
	m.selectID(id)
}

func (m *Model) flatten() {
	m.rows = m.rows[:0]
	var walk func(nodes []*tree.Node, depth int)
	walk = func(nodes []*tree.Node, depth int) {
		for _, n := range nodes {
			m.rows = append(m.rows, m.rowFor(n, depth))
			if !n.Expandable() || !m.expanded[n.ID] {
				continue
			}
			if n.Kind != kind.Folder {
				switch {
				case n.LoadErr != nil:
					m.rows = append(m.rows, row{node: n, depth: depth + 1, msg: "(cannot read: " + reason(n.LoadErr) + ")"})
					continue
				case len(n.Children) == 0:
					m.rows = append(m.rows, row{node: n, depth: depth + 1, msg: "(empty)"})
					continue
				}
			}
			walk(n.Children, depth+1)
		}
	}
	walk(m.roots, 0)
	m.rows = append(m.rows, row{plus: true})
}

func (m *Model) rowFor(n *tree.Node, depth int) row {
	r := row{node: n, depth: depth}
	switch n.Kind {
	case kind.Folder:
	case kind.Glob:
		// A glob's state is whether it matches anything.
		if !n.Loaded {
			tree.Load(n)
		}
		if len(n.Children) > 0 {
			r.info.State = fsinfo.Present
		}
	default:
		r.info = fsinfo.Stat(n.FullPath())
		if n.Kind == kind.Dir && m.expanded[n.ID] && !n.Loaded {
			tree.Load(n)
		}
	}
	return r
}

func reason(err error) string {
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i >= 0 {
		s = s[i+2:]
	}
	return s
}

func (m *Model) current() *tree.Node {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	r := m.rows[m.cursor]
	if r.msg != "" || r.plus {
		return nil
	}
	return r.node
}

func (m *Model) onPlus() bool {
	return m.cursor >= 0 && m.cursor < len(m.rows) && m.rows[m.cursor].plus
}

// selectWhere puts the cursor on the first row whose node matches.
func (m *Model) selectWhere(match func(*tree.Node) bool) {
	for i, r := range m.rows {
		if r.msg == "" && r.node != nil && match(r.node) {
			m.cursor = i
			return
		}
	}
}

func (m *Model) selectID(id string) bool {
	for i, r := range m.rows {
		if r.msg == "" && r.node != nil && r.node.ID == id {
			m.cursor = i
			return true
		}
	}
	m.setCursor(m.cursor)
	return false
}

func (m *Model) setCursor(i int) {
	m.cursor = max(0, min(i, len(m.rows)-1))
}

func (m *Model) move(delta int) { m.setCursor(m.cursor + delta) }

func (m *Model) toggle(n *tree.Node) {
	m.expanded[n.ID] = !m.expanded[n.ID]
	if m.expanded[n.ID] && n.Kind != kind.Folder {
		tree.Load(n) // re-read the directory every time it is opened
	}
	m.refreshRows()
}

func (m *Model) collapseOrParent() {
	if m.cursor >= len(m.rows) {
		return
	}
	r := m.rows[m.cursor]
	n := r.node
	if r.plus {
		return
	}
	if r.msg == "" && n.Expandable() && m.expanded[n.ID] {
		m.toggle(n)
		return
	}
	if r.msg != "" {
		m.selectID(n.ID)
		return
	}
	if n.Parent != nil {
		m.selectID(n.Parent.ID)
	}
}

func (m *Model) expandOrChild() {
	n := m.current()
	if n == nil || !n.Expandable() {
		return
	}
	if !m.expanded[n.ID] {
		m.toggle(n)
		return
	}
	m.move(1)
}

// activate opens a file or toggles anything that has children.
func (m *Model) activate(n *tree.Node) tea.Cmd {
	if n == nil {
		return nil
	}
	if n.Expandable() {
		if m.mode == modeSearch {
			m.reveal(n)
			if !m.expanded[n.ID] {
				m.toggle(n)
			}
			return nil
		}
		m.toggle(n)
		return nil
	}
	return m.openFile(n, "")
}

// ---- search ----

func (m *Model) startSearch(q string) {
	m.mode = modeSearch
	m.status = ""
	m.search.SetValue(q)
	m.search.CursorEnd()
	m.search.Focus()
	m.runSearch()
	m.cursor = 0
}

func (m *Model) runSearch() {
	q := strings.ToLower(strings.TrimSpace(m.search.Value()))
	m.rows = m.rows[:0]
	if q == "" {
		for _, n := range m.entries {
			m.rows = append(m.rows, m.rowFor(n, 0))
		}
		return
	}
	corpus := make([]string, len(m.entries))
	for i, n := range m.entries {
		corpus[i] = strings.ToLower(n.Name + " " + n.Desc + " " + fsinfo.Abbrev(n.FullPath()) + " " + n.Crumb())
	}
	for _, match := range fuzzy.Find(q, corpus) {
		m.rows = append(m.rows, m.rowFor(m.entries[match.Index], 0))
	}
}

func (m *Model) updateSearch(msg tea.KeyMsg) tea.Cmd {
	m.status = ""
	switch msg.String() {
	case "esc":
		if n := m.current(); n != nil {
			m.reveal(n)
		} else {
			m.exitSearch()
		}
		return nil
	case "enter":
		return m.activate(m.current())
	case "up", "ctrl+p":
		m.move(-1)
		return nil
	case "down", "ctrl+n":
		m.move(1)
		return nil
	case "pgup":
		m.move(-m.listHeight())
		return nil
	case "pgdown":
		m.move(m.listHeight())
		return nil
	}
	before := m.search.Value()
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	if m.search.Value() != before {
		m.runSearch()
		m.cursor, m.offset = 0, 0
	}
	return cmd
}

func (m *Model) exitSearch() {
	m.mode = modeNormal
	m.search.Blur()
	m.flatten()
	m.setCursor(m.cursor)
}

// reveal leaves search and shows n in the tree with its folders expanded.
func (m *Model) reveal(n *tree.Node) {
	for p := n.Parent; p != nil; p = p.Parent {
		m.expanded[p.ID] = true
	}
	m.exitSearch()
	m.selectID(n.ID)
}

// ---- status and dialogs ----

func (m *Model) setStatus(style lipgloss.Style, s string) {
	m.status, m.statusStyle = s, style
}

func (m *Model) fail(s string) { m.setStatus(sError, s) }

func (m *Model) openDialog(d dialog) {
	if m.mode != modeDialog {
		m.prevMode = m.mode
	}
	m.dlg = d
	m.mode = modeDialog
}

func (m *Model) closeDialog() {
	if m.mode == modeDialog {
		m.mode = m.prevMode
	}
	m.dlg = nil
}

// listHeight is the number of rows between the header and the three
// bottom lines (separator, status, keys).
func (m *Model) listHeight() int { return max(1, m.height-listTop-3) }
