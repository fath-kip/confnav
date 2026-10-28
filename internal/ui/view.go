package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/fath-kip/confnav/internal/distro"
	"github.com/fath-kip/confnav/internal/editor"
	"github.com/fath-kip/confnav/internal/fsinfo"
	"github.com/fath-kip/confnav/internal/kind"
	"github.com/fath-kip/confnav/internal/tree"
)

// View implements tea.Model.
func (m *Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	cols := m.columns()
	h := m.listHeight()

	var b strings.Builder
	b.WriteString(m.titleLine())
	b.WriteByte('\n')
	b.WriteString(m.headerLine(cols))
	b.WriteByte('\n')
	switch {
	case m.mode == modeHelp:
		var help []string
		for _, l := range strings.Split(m.helpText(), "\n") {
			help = append(help, truncStyled(l, m.width))
		}
		b.WriteString(fit(strings.Join(help, "\n"), h))
	case m.mode == modeDialog && m.dlg != nil:
		// The modal replaces the list so it can't be overlooked.
		b.WriteString(fit(lipgloss.Place(m.width, h, lipgloss.Center, lipgloss.Center, m.dlg.view(m.width)), h))
	default:
		b.WriteString(m.listView(cols, h))
	}
	b.WriteByte('\n')
	b.WriteString(m.bottomView())
	return b.String()
}

func (m *Model) titleLine() string {
	var left, right string
	if m.mode == modeSearch || (m.mode == modeDialog && m.prevMode == modeSearch) {
		m.search.Width = max(10, m.width-20)
		left = m.search.View()
		right = sFaint.Render(fmt.Sprintf("%d found", len(m.rows)))
	} else {
		name := m.opt.Distro.Name
		if name == "" {
			name = "unknown system"
		}
		fam := string(m.opt.Family)
		if m.opt.Family == distro.Unknown {
			fam = "generic"
		}
		left = sBold.Render("confnav") + "  " + sFaint.Render(name+" · "+fam)
		right = keyHelp(m.keys.label("search"), "search", m.keys.label("help"), "help")
		if m.missingHidden > 0 {
			right = sFaint.Render(fmt.Sprintf("%d missing hidden", m.missingHidden)) + sFaint.Render(" · ") + right
		}
	}
	space := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if space < 1 {
		return left
	}
	return left + strings.Repeat(" ", space) + right
}

func (m *Model) headerLine(c columns) string {
	return sHeader.Render(c.join(pad("NAME", c.name), pad("DESCRIPTION", c.desc), pad("PATH", c.path), pad("MODIFIED", c.mod)))
}

// columns sizes the columns from the widest values of all rows, not just
// the visible ones, so the layout does not jump while scrolling.
func (m *Model) columns() columns {
	nName, nDesc, nPath := width("NAME"), width("DESCRIPTION"), width("PATH")
	for _, r := range m.rows {
		if r.msg != "" || r.plus || (r.node.Kind == kind.Folder && m.mode != modeSearch) {
			continue
		}
		nName = max(nName, width(m.nameCell(r)))
		nDesc = max(nDesc, width(r.node.Desc))
		nPath = max(nPath, width(fsinfo.Abbrev(r.node.FullPath())))
	}
	return layout(m.width, nName, nDesc, nPath)
}

func (m *Model) listView(c columns, h int) string {
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	m.offset = max(0, min(m.offset, len(m.rows)-h))

	lines := make([]string, 0, h)
	for i := m.offset; i < len(m.rows) && len(lines) < h; i++ {
		lines = append(lines, m.renderRow(m.rows[i], c, i == m.cursor))
	}
	if len(m.rows) == 0 {
		msg := "Nothing here."
		if m.mode == modeSearch {
			msg = "No matches."
		}
		lines = append(lines, sFaint.Render(msg))
	}
	return fit(strings.Join(lines, "\n"), h)
}

func (m *Model) marker(n *tree.Node) string {
	switch {
	case !n.Expandable():
		return "  "
	case m.expanded[n.ID] && m.mode != modeSearch:
		return "▾ "
	}
	return "▸ "
}

// nameCell is the indented name with its expand marker.
func (m *Model) nameCell(r row) string {
	name := r.node.Name
	if r.node.Kind == kind.Dir && !strings.HasSuffix(name, "/") {
		name += "/"
	}
	return strings.Repeat("  ", r.depth) + m.marker(r.node) + name
}

func (m *Model) renderRow(r row, c columns, selected bool) string {
	style := func(s string, st interface{ Render(...string) string }) string {
		if selected {
			return s
		}
		return st.Render(s)
	}
	finish := func(s string) string {
		if selected {
			return sSelected.Render(pad(s, m.width))
		}
		return s
	}

	if r.plus {
		return finish(style("+", sFolder))
	}
	if r.msg != "" {
		return finish(style(pad(strings.Repeat("  ", r.depth)+"  "+r.msg, m.width), sFaint))
	}
	n := r.node
	if n.Kind == kind.Folder && m.mode != modeSearch {
		label := strings.Repeat("  ", r.depth) + m.marker(n)
		if n.Origin == tree.User {
			label += "★ "
		}
		label += n.Name
		count := fmt.Sprintf(" (%d)", len(n.Children))
		label = truncEnd(label, m.width-width(count))
		return finish(style(label, sFolder) + style(count, sFaint))
	}

	missing := r.info.State == fsinfo.Missing && n.Kind != kind.Glob
	cell := func(s string, st interface{ Render(...string) string }) string {
		if missing {
			return style(s, sFaint)
		}
		return style(s, st)
	}
	nameStyle := sPlain
	if n.Expandable() {
		nameStyle = sDir
	}
	return finish(c.join(
		cell(pad(m.nameCell(r), c.name), nameStyle),
		cell(pad(n.Desc, c.desc), sPlain),
		cell(pad(truncMiddle(fsinfo.Abbrev(n.FullPath()), c.path), c.path), sPath),
		cell(pad(m.modText(r), c.mod), sFaint),
	))
}

func (m *Model) modText(r row) string {
	if r.node.Kind == kind.Glob {
		switch k := len(r.node.Children); k {
		case 0:
			return "no match"
		case 1:
			return "1 match"
		default:
			return fmt.Sprintf("%d matches", k)
		}
	}
	switch r.info.State {
	case fsinfo.Missing:
		return "missing"
	case fsinfo.NoAccess:
		return "no access"
	}
	return r.info.ModTime.Format(m.cfg.DateFormat)
}

func (m *Model) bottomView() string {
	return sFaint.Render(strings.Repeat("─", m.width)) + "\n" +
		truncStyled(m.statusLine(), m.width) + "\n" +
		truncStyled(m.keysLine(), m.width)
}

func (m *Model) statusLine() string {
	if m.status != "" {
		return m.statusStyle.Render(truncEnd(m.status, m.width))
	}
	if m.mode == modeDialog {
		return "" // the row behind the modal is not what the user looks at
	}
	if m.onPlus() && m.mode == modeNormal {
		return sFaint.Render(fmt.Sprintf("Add a folder or file at the top level (%s or %s)",
			m.keys.label("open"), m.keys.label("add")))
	}
	n := m.current()
	if n == nil {
		return ""
	}
	if n.Kind == kind.Folder {
		switch n.Origin {
		case tree.Recent:
			return sFaint.Render("Files you opened recently")
		case tree.User:
			return sFaint.Render(fmt.Sprintf("Your folder · %d item(s) · %s adds a file or folder here",
				len(n.Children), m.keys.label("add")))
		}
		return sFaint.Render(fmt.Sprintf("Built-in category · %d item(s)", len(n.Children)))
	}
	parts := []string{sPath.Render(n.FullPath())}
	if m.mode == modeSearch {
		parts = append(parts, sFaint.Render("in "+n.Crumb()))
	}
	if !n.Opened.IsZero() {
		parts = append(parts, sFaint.Render("opened "+n.Opened.Format(m.cfg.DateFormat)))
	}
	if n.Defaults != "" && fsinfo.Stat(n.FullPath()).State == fsinfo.Missing &&
		fsinfo.Stat(n.Defaults).State == fsinfo.Present {
		parts = append(parts, sFaint.Render("defaults in "+n.Defaults))
	}
	if tool := editor.Tool(n.FullPath()); tool != "" {
		parts = append(parts, sFaint.Render("opens with "+tool))
	}
	if n.Hint != "" {
		parts = append(parts, sHint.Render("after edit: "+n.Hint))
	}
	return strings.Join(parts, sFaint.Render(" · "))
}

func (m *Model) keysLine() string {
	switch m.mode {
	case modeSearch:
		return keyHelp("enter", "open", "↑↓", "move", "esc", "show in tree", "ctrl+c", "quit")
	case modeHelp:
		return keyHelp("any key", "close help")
	case modeDialog:
		return "" // the modal shows its own keys
	}
	missing := "hide missing"
	if !m.cfg.ShowMissing {
		missing = "show missing"
	}
	var pairs []string
	for _, a := range [][2]string{
		{"open", "open"}, {"add", "add"}, {"edit", "edit"}, {"remove", "remove"},
		{"search", "search"}, {"editor", "editor"}, {"settings", "settings"},
		{"toggle_missing", missing}, {"help", "help"}, {"quit", "quit"},
	} {
		if k := m.keys.label(a[0]); k != "" {
			pairs = append(pairs, k, a[1])
		}
	}
	return keyHelp(pairs...)
}

func (m *Model) helpText() string {
	type item struct{ action, desc string }
	settingsKey := m.keys.label("settings")
	if settingsKey == "" {
		settingsKey = "the settings action"
	}
	sections := []struct {
		title string
		items []item
		note  string
	}{
		{"Move", []item{{"up", "up"}, {"down", "down"}, {"collapse", "collapse or go to parent"},
			{"expand", "expand"}, {"top", "top"}, {"bottom", "bottom"}, {"page_up", "page up"},
			{"page_down", "page down"}}, ""},
		{"Open", []item{{"open", "open file or toggle folder"}, {"open_with", "open once with another editor"},
			{"editor", "set default editor"}, {"settings", "open confnav's settings (config.toml)"}},
			"Files you can't write open with sudoedit; sudoers, passwd and group open with visudo, vipw and vigr."},
		{"Organise", []item{{"add", "add a file, directory, glob or folder (the + row adds at the top level)"},
			{"edit", "rename / edit"}, {"copy", "copy to folder"}, {"move", "move to folder"},
			{"remove", "remove (built-ins are hidden)"}, {"restore", "restore hidden built-ins"},
			{"toggle_missing", "show or hide files that don't exist on this system"}}, ""},
		{"Other", []item{{"search", "search everything"}, {"yank", "copy path to clipboard"},
			{"refresh", "refresh and reload settings"}, {"help", "help"}, {"quit", "quit"}},
			"Change any key in config.toml ([keys]; " + settingsKey + " opens it); saving applies it at once."},
		{"Files", nil, "confnav never deletes or changes files on disk; removing only affects its own lists."},
		{"Mouse", nil, "Click a folder to toggle it, double-click a file or the + row to open it, scroll with the wheel."},
	}
	const titleW = 9
	body := lipgloss.NewStyle().Width(max(20, m.width-titleW-4))
	indent := strings.Repeat(" ", titleW+4)
	lines := []string{sBold.Render("Keys"), ""}
	add := func(title, text string) {
		for i, l := range strings.Split(body.Render(text), "\n") {
			if i == 0 {
				lines = append(lines, "  "+sFolder.Render(pad(title, titleW))+"  "+l)
			} else {
				lines = append(lines, indent+l)
			}
		}
	}
	for _, sec := range sections {
		var parts []string
		for _, it := range sec.items {
			if k := m.keys.labels(it.action); k != "" {
				parts = append(parts, sBold.Render(k)+" "+it.desc)
			}
		}
		title := sec.title
		if len(parts) > 0 {
			add(title, strings.Join(parts, sFaint.Render(" · ")))
			title = ""
		}
		if sec.note != "" {
			add(title, sFaint.Render(sec.note))
		}
	}
	return strings.Join(lines, "\n")
}

// truncStyled cuts a string that already contains colour codes.
func truncStyled(s string, w int) string { return ansi.Truncate(s, w, "…") }

// fit pads or cuts s to exactly h lines.
func fit(s string, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
