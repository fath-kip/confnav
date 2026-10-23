package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/fath-kip/confnav/internal/fsinfo"
)

// dialog is a modal shown in the middle of the screen that takes all keys
// until it closes. Dialogs close themselves (m.closeDialog) before running
// their callback, so a callback may open the next dialog.
type dialog interface {
	update(m *Model, msg tea.KeyMsg) tea.Cmd
	// view renders the modal for a screen of width w.
	view(w int) string
}

// modalInner is the text width inside a modal on a screen of width w that
// wants content cells: wide enough to stand out, never wider than the screen.
func modalInner(w, content int) int {
	outer := max(24, min(w-4, max(64, content+8)))
	return outer - 2 - 4 // border and horizontal padding
}

// modal draws a dialog: a coloured title bar, an optional subtitle, the
// body lines and a line of key help.
func modal(inner int, title, subtitle string, body []string, help string) string {
	lines := []string{sModalTitle.Width(inner).Render(truncEnd(title, inner-2))}
	if subtitle != "" {
		lines = append(lines, "", sBold.Width(inner).Render(subtitle))
	}
	lines = append(lines, "")
	lines = append(lines, body...)
	lines = append(lines, "", help)
	return sModal.Width(inner + 4).Render(strings.Join(lines, "\n"))
}

func keyHelp(pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, sBold.Render(pairs[i])+" "+sFaint.Render(pairs[i+1]))
	}
	return strings.Join(parts, sFaint.Render(" · "))
}

// ---- form ----

type fieldSpec struct {
	label, value, placeholder string
	path                      bool // Tab completes file system paths
}

type field struct {
	label string
	in    textinput.Model
	path  bool
}

type form struct {
	title  string
	fields []field
	focus  int
	note   string
	isErr  bool
	submit func(m *Model, vals []string) (tea.Cmd, error)
}

func newForm(title string, submit func(*Model, []string) (tea.Cmd, error), specs ...fieldSpec) *form {
	f := &form{title: title, submit: submit}
	for _, s := range specs {
		in := textinput.New()
		in.Prompt = ""
		in.Placeholder = s.placeholder
		in.SetValue(s.value)
		in.CursorEnd()
		in.Cursor.SetMode(cursor.CursorStatic)
		f.fields = append(f.fields, field{label: s.label, in: in, path: s.path})
	}
	f.setFocus(0)
	return f
}

func (f *form) setFocus(i int) {
	f.focus = (i + len(f.fields)) % len(f.fields)
	for j := range f.fields {
		if j == f.focus {
			f.fields[j].in.Focus()
		} else {
			f.fields[j].in.Blur()
		}
	}
}

func (f *form) update(m *Model, msg tea.KeyMsg) tea.Cmd {
	cur := &f.fields[f.focus]
	switch msg.String() {
	case "esc":
		m.closeDialog()
		return nil
	case "up", "shift+tab":
		f.setFocus(f.focus - 1)
		return nil
	case "down":
		f.setFocus(f.focus + 1)
		return nil
	case "tab":
		if !cur.path {
			f.setFocus(f.focus + 1)
			return nil
		}
		completed, cands := fsinfo.Complete(cur.in.Value())
		cur.in.SetValue(completed)
		cur.in.CursorEnd()
		f.note, f.isErr = strings.Join(cands, "  "), false
		return nil
	case "enter":
		if f.focus < len(f.fields)-1 {
			f.setFocus(f.focus + 1)
			return nil
		}
		vals := make([]string, len(f.fields))
		for i, fl := range f.fields {
			vals[i] = strings.TrimSpace(fl.in.Value())
		}
		m.closeDialog()
		cmd, err := f.submit(m, vals)
		if err != nil {
			f.note, f.isErr = err.Error(), true
			m.openDialog(f)
		}
		return cmd
	}
	var cmd tea.Cmd
	cur.in, cmd = cur.in.Update(msg)
	return cmd
}

func (f *form) view(w int) string {
	labelW := 0
	for _, fl := range f.fields {
		labelW = max(labelW, width(fl.label))
	}
	inner := modalInner(w, labelW+56)
	var body []string
	for i := range f.fields {
		fl := &f.fields[i]
		fl.in.Width = max(10, inner-labelW-3)
		label := pad(fl.label, labelW)
		if i == f.focus {
			label = sBold.Render(label)
		} else {
			label = sFaint.Render(label)
		}
		body = append(body, label+"  "+fl.in.View())
	}
	if f.note != "" {
		style := sFaint
		if f.isErr {
			style = sError
		}
		body = append(body, "", style.Width(inner).Render(f.note))
	}
	help := []string{"enter", "next/save", "↑↓", "field", "esc", "cancel"}
	if f.fields[f.focus].path {
		help = append([]string{"tab", "complete"}, help...)
	}
	return modal(inner, f.title, "", body, keyHelp(help...))
}

// ---- confirm ----

type confirm struct {
	title, body string
	yes         func(m *Model) tea.Cmd
}

func (c *confirm) update(m *Model, msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "y", "Y":
		m.closeDialog()
		return c.yes(m)
	case "n", "N", "esc", "q":
		m.closeDialog()
	}
	return nil
}

func (c *confirm) view(w int) string {
	inner := modalInner(w, width(c.body))
	var body []string
	if c.body != "" {
		body = append(body, sPlain.Width(inner).Render(c.body))
	}
	return modal(inner, c.title, "", body, keyHelp("y", "yes", "n", "no"))
}

// ---- picker ----

const pickerRows = 10

type picker struct {
	title    string
	subtitle string
	items    []string
	sel      int
	pick     func(m *Model, i int) tea.Cmd
	cancel   func(m *Model)
}

func (p *picker) update(m *Model, msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "up", "k":
		p.sel = max(0, p.sel-1)
	case "down", "j":
		p.sel = min(len(p.items)-1, p.sel+1)
	case "home", "g":
		p.sel = 0
	case "end", "G":
		p.sel = len(p.items) - 1
	case "enter":
		m.closeDialog()
		return p.pick(m, p.sel)
	case "esc", "q":
		m.closeDialog()
		if p.cancel != nil {
			p.cancel(m)
		}
	}
	return nil
}

func (p *picker) view(w int) string {
	content := width(p.subtitle) / 2
	for _, it := range p.items {
		content = max(content, width(it)+4)
	}
	inner := modalInner(w, content)
	start := max(0, min(p.sel-pickerRows/2, len(p.items)-pickerRows))
	end := min(len(p.items), start+pickerRows)
	var body []string
	if start > 0 {
		body = append(body, sFaint.Render("  ↑ more"))
	}
	for i := start; i < end; i++ {
		if i == p.sel {
			body = append(body, sPicked.Width(inner).Render("› "+truncEnd(p.items[i], inner-2)))
		} else {
			body = append(body, "  "+truncEnd(p.items[i], inner-2))
		}
	}
	if end < len(p.items) {
		body = append(body, sFaint.Render("  ↓ more"))
	}
	return modal(inner, p.title, p.subtitle, body, keyHelp("↑↓", "choose", "enter", "select", "esc", "cancel"))
}
