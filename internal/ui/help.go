package ui

import (
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fath-kip/confnav/internal/config"
	"github.com/fath-kip/confnav/internal/fsinfo"
	"github.com/fath-kip/confnav/internal/recent"
	"github.com/fath-kip/confnav/internal/xdg"
)

// helpItem is one line of a help box: keys and what they do.
type helpItem struct{ keys, desc string }

type helpSection struct {
	title string
	items []helpItem
	note  string
}

// actions turns action names into help items with their current keys;
// actions without keys are left out.
func (m *Model) actions(names ...string) []helpItem {
	var out []helpItem
	for _, a := range names {
		if k := m.keys.labels(a); k != "" {
			out = append(out, helpItem{k, config.KeyDesc(a)})
		}
	}
	return out
}

func (m *Model) helpSections() []helpSection {
	settings := m.keys.label("settings")
	if settings == "" {
		settings = "the settings action"
	}
	return []helpSection{
		{"Move", m.actions("up", "down", "collapse", "expand", "top", "bottom", "page_up", "page_down"), ""},
		{"Open", m.actions("open", "open_with", "editor", "settings"),
			"Files you can't write open with sudoedit, so your editor runs as you. " +
				"sudoers, passwd and group open with visudo, vipw and vigr."},
		{"Organise", m.actions("add", "edit", "copy", "move", "remove", "toggle_missing"),
			"The + row at the bottom adds at the top level. Everything in the list is stored under " +
				"[category_structure] in config.toml, so you can edit it there too."},
		{"Find and set up", m.actions("search", "scan", "preferences", "yank", "refresh", "help", "quit"),
			"Change any key under [keys] in config.toml (" + settings + " opens it); saving applies it at once."},
		{"Search", []helpItem{{"type", "filter names, descriptions and paths"}, {"enter", "open"},
			{"↑↓", "move"}, {"esc", "show the result in the tree"}}, ""},
		{"Mouse", []helpItem{{"click", "toggle a folder"}, {"double-click", "open a file or the + row"},
			{"wheel", "scroll"}}, "Hold Shift to select text with the mouse."},
		{"Files", []helpItem{
			{"settings", fsinfo.Abbrev(m.opt.ConfigPath)},
			{"recent", fsinfo.Abbrev(recent.Path())},
			{"backups", fsinfo.Abbrev(filepath.Join(xdg.StateDir(), "backups"))},
		}, "confnav never deletes or changes files on disk; removing only affects its own list."},
	}
}

// helpBand is the coloured line above the help page.
func (m *Model) helpBand() string {
	title := "Help"
	if m.opt.Version != "" {
		title += " · confnav " + m.opt.Version
	}
	return sModalTitle.Width(m.width).Render(truncEnd(title, m.width-2))
}

// helpBox renders one section as a framed box w cells wide.
func helpBox(sec helpSection, w int) string {
	inner := max(10, w-4) // border and padding
	keyW := 0
	for _, it := range sec.items {
		keyW = max(keyW, width(it.keys))
	}
	keyW = min(keyW, inner/3)
	descW := max(8, inner-keyW-2)
	lines := []string{sFolder.Render(sec.title)}
	for _, it := range sec.items {
		desc := strings.Split(lipgloss.NewStyle().Width(descW).Render(it.desc), "\n")
		lines = append(lines, sKey.Render(pad(truncEnd(it.keys, keyW), keyW))+"  "+desc[0])
		for _, d := range desc[1:] {
			lines = append(lines, strings.Repeat(" ", keyW+2)+d)
		}
	}
	if sec.note != "" {
		lines = append(lines, "", sFaint.Width(inner).Render(sec.note))
	}
	return sBox.Width(w - 2).Render(strings.Join(lines, "\n"))
}

// helpLines lays the boxes out in two columns on wide screens, one otherwise.
func (m *Model) helpLines() []string {
	boxes := m.helpSections()
	if m.width < 100 {
		var out []string
		for _, sec := range boxes {
			out = append(out, strings.Split(helpBox(sec, m.width), "\n")...)
		}
		return out
	}
	colW := (m.width - 1) / 2
	var cols [2][]string
	for _, sec := range boxes {
		// Fill the shorter column, so both end at about the same height.
		c := 0
		if len(cols[1]) < len(cols[0]) {
			c = 1
		}
		cols[c] = append(cols[c], strings.Split(helpBox(sec, colW), "\n")...)
	}
	left := lipgloss.NewStyle().Width(colW).Render(strings.Join(cols[0], "\n"))
	right := strings.Join(cols[1], "\n")
	return strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right), "\n")
}

// helpView shows h lines of the help page from the current scroll position.
func (m *Model) helpView(h int) string {
	lines := m.helpLines()
	m.helpOffset = max(0, min(m.helpOffset, len(lines)-h))
	end := min(len(lines), m.helpOffset+h)
	view := lines[m.helpOffset:end]
	if end < len(lines) && len(view) > 0 {
		view[len(view)-1] = sFaint.Render(pad("  ↓ more below", m.width))
	}
	for i, l := range view {
		view[i] = truncStyled(l, m.width)
	}
	return fit(strings.Join(view, "\n"), h)
}

// updateHelp scrolls the help page; any other key closes it.
func (m *Model) updateHelp(msg tea.KeyMsg) {
	h := m.listHeight()
	switch msg.String() {
	case "up", "k":
		m.helpOffset--
	case "down", "j":
		m.helpOffset++
	case "pgup", "ctrl+b":
		m.helpOffset -= h
	case "pgdown", "ctrl+f", " ":
		m.helpOffset += h
	case "home", "g":
		m.helpOffset = 0
	case "end", "G":
		m.helpOffset = 1 << 20 // helpView clamps it
	default:
		m.mode = modeNormal
	}
}
