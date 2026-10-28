package ui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/aymanbagabas/go-osc52/v2"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/fath-kip/confnav/internal/config"
	"github.com/fath-kip/confnav/internal/editor"
	"github.com/fath-kip/confnav/internal/fsinfo"
	"github.com/fath-kip/confnav/internal/kind"
	"github.com/fath-kip/confnav/internal/tree"
	"github.com/fath-kip/confnav/internal/xdg"
)

// ---- opening files ----

// copyDoneMsg reports that `sudo cp` of the vendor defaults finished.
type copyDoneMsg struct {
	node *tree.Node
	path string
	ed   string
	err  error
}

type editorDoneMsg struct {
	path      string
	hint      string
	before    fsinfo.Info
	backupErr error
	err       error
}

func (m *Model) openFile(n *tree.Node, ed string) tea.Cmd {
	path := n.FullPath()
	info := fsinfo.Stat(path)
	if info.State == fsinfo.Missing && path == m.opt.ConfigPath {
		if err := config.Save(path, m.cfg); err != nil {
			m.fail("Could not create config: " + err.Error())
			return nil
		}
		info = fsinfo.Stat(path)
	}
	if info.State == fsinfo.Missing {
		dir := filepath.Dir(path)
		if fsinfo.Stat(dir).State == fsinfo.Missing {
			m.fail("Directory " + fsinfo.Abbrev(dir) + " does not exist, so the file can't be created")
			return nil
		}
		if n.Defaults != "" && fsinfo.Stat(n.Defaults).State == fsinfo.Present {
			m.openDialog(&picker{
				title: "Create " + fsinfo.Abbrev(path) + "? It does not exist yet.",
				items: []string{"Start from the distribution defaults in " + n.Defaults, "Start with an empty file"},
				pick: func(m *Model, i int) tea.Cmd {
					if i == 1 {
						return m.launch(n, path, ed, info)
					}
					return m.copyDefaults(n, path, ed)
				},
			})
			return nil
		}
		m.openDialog(&confirm{
			title: "Create " + fsinfo.Abbrev(path) + "?",
			body:  "The file does not exist yet. It is created when you save in the editor.",
			yes:   func(m *Model) tea.Cmd { return m.launch(n, path, ed, info) },
		})
		return nil
	}
	return m.launch(n, path, ed, info)
}

// copyDefaults copies the vendor file into place, through sudo when the
// target directory is not writable, then opens the copy. The vendor file
// itself is never edited: package updates would overwrite the changes.
func (m *Model) copyDefaults(n *tree.Node, path, ed string) tea.Cmd {
	if fsinfo.Writable(path) {
		err := fsinfo.CopyFile(n.Defaults, path, 0o644)
		return func() tea.Msg { return copyDoneMsg{node: n, path: path, ed: ed, err: err} }
	}
	cmd := exec.Command("sudo", "cp", n.Defaults, path)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return copyDoneMsg{node: n, path: path, ed: ed, err: err}
	})
}

func (m *Model) afterCopy(msg copyDoneMsg) tea.Cmd {
	if msg.err != nil {
		m.fail("Could not copy the defaults: " + msg.err.Error())
		return nil
	}
	return m.launch(msg.node, msg.path, msg.ed, fsinfo.Info{State: fsinfo.Missing})
}

func (m *Model) launch(n *tree.Node, path, ed string, before fsinfo.Info) tea.Cmd {
	if ed == "" {
		ed = editor.Resolve(m.cfg.Editor)
	}
	done := editorDoneMsg{path: path, hint: n.Hint, before: before}
	if m.cfg.Backup && before.State == fsinfo.Present {
		_, done.backupErr = fsinfo.Backup(path, filepath.Join(xdg.StateDir(), "backups"), m.now())
	}
	l, err := editor.Prepare(ed, path)
	if err != nil {
		m.fail(err.Error())
		return nil
	}
	return tea.ExecProcess(l.Cmd, func(err error) tea.Msg {
		done.err = err
		return done
	})
}

func (m *Model) afterEdit(msg editorDoneMsg) tea.Cmd {
	short := fsinfo.Abbrev(msg.path)
	if msg.err != nil {
		m.rebuild()
		m.fail("Editor failed for " + short + ": " + msg.err.Error())
		return nil
	}
	m.recent.Add(msg.path, m.now())
	saveErr := m.recent.Save()
	m.rebuild()

	after := fsinfo.Stat(msg.path)
	changed := after.State == fsinfo.Present &&
		(msg.before.State != fsinfo.Present || !after.ModTime.Equal(msg.before.ModTime))
	switch {
	case !changed:
		m.setStatus(sFaint, "No changes to "+short)
	case msg.hint != "":
		m.setStatus(sHint, "Saved "+short+" · next: "+msg.hint)
	default:
		m.setStatus(sOK, "Saved "+short)
	}
	if msg.backupErr != nil {
		m.fail(m.status + " (backup failed: " + reason(msg.backupErr) + ")")
	}
	if saveErr != nil {
		m.fail("Could not save recent list: " + saveErr.Error())
	}
	if msg.path != m.opt.ConfigPath || !changed {
		return nil
	}
	cmd, err := m.reloadConfig()
	if err != nil {
		m.fail(err.Error())
	}
	return cmd
}

// reloadConfig re-reads config.toml and applies it in place, which is
// cheaper than restarting and needs nothing from the user. A broken file
// keeps the current settings.
func (m *Model) reloadConfig() (tea.Cmd, error) {
	cfg, _, err := config.Load(m.opt.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("config.toml: %v (previous settings kept)", err)
	}
	var cmd tea.Cmd
	if cfg.Mouse != m.cfg.Mouse {
		cmd = tea.DisableMouse
		if cfg.Mouse {
			cmd = tea.EnableMouseCellMotion
		}
	}
	changed := !reflect.DeepEqual(cfg, m.cfg)
	m.cfg = cfg
	m.recent.Limit = cfg.RecentLimit
	var warns []string
	m.keys, warns = buildKeymap(cfg.Keys)
	m.rebuild()
	switch {
	case len(warns) > 0:
		m.setStatus(sHint, "Settings reloaded; config.toml keys: "+strings.Join(warns, "; "))
	case changed:
		m.setStatus(sOK, "Settings reloaded")
	}
	return cmd, nil
}

// toggleMissing shows or hides built-in entries that don't exist here.
func (m *Model) toggleMissing() {
	m.cfg.ShowMissing = !m.cfg.ShowMissing
	m.rebuild()
	msg := "Showing files that don't exist on this system"
	if !m.cfg.ShowMissing {
		msg = fmt.Sprintf("Hid %d file(s) that don't exist on this system (%s shows them again)",
			m.missingHidden, m.keys.label("toggle_missing"))
	}
	if err := config.Save(m.opt.ConfigPath, m.cfg); err != nil {
		m.fail("Could not save config: " + err.Error())
		return
	}
	m.setStatus(sOK, msg)
}

// openWith opens the file once with another editor.
func (m *Model) openWith(n *tree.Node) {
	if n == nil || n.Kind != kind.File {
		m.fail("Select a file to open")
		return
	}
	items := append(editor.Detect(), "Custom command…")
	m.openDialog(&picker{
		title: "Open " + n.Name + " with",
		items: items,
		pick: func(m *Model, i int) tea.Cmd {
			if i < len(items)-1 {
				return m.openFile(n, items[i])
			}
			m.openDialog(newForm("Open "+n.Name+" with", func(m *Model, v []string) (tea.Cmd, error) {
				if v[0] == "" {
					return nil, errors.New("enter a command")
				}
				return m.openFile(n, v[0]), nil
			}, fieldSpec{label: "Command", placeholder: "e.g. code --wait"}))
			return nil
		},
	})
}

// editorPicker asks which editor opens files and passes the answer to done.
func (m *Model) editorPicker(title, subtitle, note string, done func(m *Model, ed string) tea.Cmd, cancel func(m *Model)) {
	current := editor.Resolve(m.cfg.Editor)
	items := editor.Detect()
	sel := 0
	labels := make([]string, len(items))
	for i, e := range items {
		labels[i] = e
		if e == current {
			sel = i
			labels[i] += "  (current)"
		}
	}
	labels = append(labels, "Custom command…")
	m.openDialog(&picker{
		title:    title,
		subtitle: subtitle,
		note:     note,
		items:    labels,
		sel:      sel,
		pick: func(m *Model, i int) tea.Cmd {
			if i < len(items) {
				return done(m, items[i])
			}
			f := newForm(title, func(m *Model, v []string) (tea.Cmd, error) {
				if v[0] == "" {
					return nil, errors.New("enter a command")
				}
				return done(m, v[0]), nil
			}, fieldSpec{label: "Command", value: m.cfg.Editor, placeholder: "e.g. nvim -p"})
			f.cancel = cancel
			m.openDialog(f)
			return nil
		},
		cancel: cancel,
	})
}

// chooseEditor sets the default editor.
func (m *Model) chooseEditor() {
	m.editorPicker("Default editor", "Which editor should open your files?", "", func(m *Model, ed string) tea.Cmd {
		m.cfg.Editor = ed
		if err := config.Save(m.opt.ConfigPath, m.cfg); err != nil {
			m.fail("Could not save config: " + err.Error())
			return nil
		}
		m.setStatus(sOK, "Files now open with "+ed+" (change with "+m.keys.label("editor")+")")
		return nil
	}, nil)
}

// openSettings opens confnav's own config.toml; saving it applies the
// changes at once (see afterEdit).
func (m *Model) openSettings() tea.Cmd {
	n := &tree.Node{Kind: kind.File, Name: "config.toml", Path: m.opt.ConfigPath,
		Hint: "changes apply as soon as you save"}
	return m.openFile(n, "")
}

// ---- organising ----

// target says where new nodes go: a folder of the tree, or nil for the top level.
type target struct {
	folder *tree.Node
}

func (t target) name() string {
	if t.folder == nil {
		return "the top level"
	}
	return t.folder.Name
}

// container is the config folder for t (nil: the top level).
func (t target) container() *config.Folder {
	if t.folder == nil {
		return nil
	}
	return t.folder.Folder
}

// targetFor finds where new nodes go for the selected node: its folder,
// or the top level for top-level entries, Recent and the + row.
func targetFor(n *tree.Node) target {
	if n != nil {
		if f := n.FolderNode(); f != nil {
			if _, ok := f.Container(); ok {
				return target{f}
			}
		}
	}
	return target{}
}

// addPrompt asks whether to add a file or a folder next to n (nil: top level).
func (m *Model) addPrompt(n *tree.Node) {
	t := targetFor(n)
	m.openDialog(&picker{
		title:    "Add to " + t.name(),
		subtitle: "What do you want to add?",
		items:    []string{"File, directory or glob", "Folder"},
		pick: func(m *Model, i int) tea.Cmd {
			if i == 0 {
				m.addEntry(t)
			} else {
				m.newFolder(t)
			}
			return nil
		},
	})
}

func (m *Model) addEntry(t target) {
	m.openDialog(newForm("Add a file to "+t.name(), func(m *Model, v []string) (tea.Cmd, error) {
		p := cleanPath(v[0])
		if p == "" {
			return nil, errors.New("a path is required")
		}
		full := fsinfo.Expand(p)
		if !filepath.IsAbs(full) {
			return nil, errors.New("the path must be absolute or start with ~/")
		}
		name := v[1]
		if name == "" {
			name = filepath.Base(full)
		}
		e := &config.Entry{Name: name, Desc: v[2], Path: p, Kind: kind.Infer(full)}
		m.cfg.Structure.AddEntry(t.container(), e)
		if t.folder != nil {
			m.expanded[t.folder.ID] = true
		}
		m.saveConfig("Added " + name)
		m.selectWhere(func(n *tree.Node) bool { return n.Entry == e })
		return nil, nil
	},
		fieldSpec{label: "Path", placeholder: "/etc/… or ~/…  (a directory or a glob like *.conf works too)", path: true},
		fieldSpec{label: "Name", placeholder: "default: the file name"},
		fieldSpec{label: "Description", placeholder: "optional"},
	))
}

func cleanPath(p string) string {
	p = strings.TrimSpace(p)
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	return p
}

func (m *Model) newFolder(t target) {
	m.openDialog(newForm("New folder in "+t.name(), func(m *Model, v []string) (tea.Cmd, error) {
		if v[0] == "" {
			return nil, errors.New("a name is required")
		}
		f := &config.Folder{Name: v[0]}
		m.cfg.Structure.AddFolder(t.container(), f)
		if t.folder != nil {
			m.expanded[t.folder.ID] = true
		}
		m.saveConfig("Created folder " + v[0])
		m.selectWhere(func(n *tree.Node) bool { return n.Folder == f })
		return nil, nil
	}, fieldSpec{label: "Name", placeholder: "e.g. Daily"}))
}

func (m *Model) editNode(n *tree.Node) {
	switch {
	case n == nil:
		return
	case n.Origin != tree.Config:
		m.fail("Only entries of your list can be edited; copy this one to a folder with " + m.keys.label("copy"))
		return
	case n.Folder != nil:
		f := n.Folder
		m.openDialog(newForm("Rename "+f.Name, func(m *Model, v []string) (tea.Cmd, error) {
			if v[0] == "" {
				return nil, errors.New("a name is required")
			}
			f.Name = v[0]
			m.saveConfig("Renamed to " + v[0])
			return nil, nil
		}, fieldSpec{label: "Name", value: f.Name}))
		return
	}
	e := n.Entry
	m.openDialog(newForm("Edit "+e.Name, func(m *Model, v []string) (tea.Cmd, error) {
		p := cleanPath(v[0])
		full := fsinfo.Expand(p)
		if p == "" || !filepath.IsAbs(full) {
			return nil, errors.New("the path must be absolute or start with ~/")
		}
		if v[1] == "" {
			v[1] = filepath.Base(full)
		}
		e.Scanned = e.Scanned && p == e.Path
		e.Path, e.Name, e.Desc, e.Kind = p, v[1], v[2], kind.Infer(full)
		m.saveConfig("Updated " + e.Name)
		return nil, nil
	},
		fieldSpec{label: "Path", value: e.Path, path: true},
		fieldSpec{label: "Name", value: e.Name},
		fieldSpec{label: "Description", value: e.Desc},
	))
}

// copyOrMove puts n into another folder. Recent items and files inside a
// listed directory can only be copied: they become entries of their own.
func (m *Model) copyOrMove(n *tree.Node, move bool) {
	if n == nil || n.ID == tree.RecentID {
		m.fail("Select an entry or folder")
		return
	}
	move = move && n.Origin == tree.Config
	verb := "Copy"
	if move {
		verb = "Move"
	}
	var dests []*tree.Node // nil: the top level
	var labels []string
	var walk func([]*tree.Node)
	walk = func(nodes []*tree.Node) {
		for _, f := range nodes {
			if f.Kind != kind.Folder || (move && f == n) {
				continue
			}
			if _, ok := f.Container(); ok && f != n.Parent {
				label := f.Name
				if crumb := f.Crumb(); crumb != "" {
					label = crumb + " › " + f.Name
				}
				dests = append(dests, f)
				labels = append(labels, label)
			}
			walk(f.Children)
		}
	}
	walk(m.roots)
	if n.Parent != nil {
		dests = append(dests, nil)
		labels = append(labels, "(top level)")
	}
	if len(dests) == 0 {
		m.fail("There is no other folder; create one with " + m.keys.label("add"))
		return
	}
	m.openDialog(&picker{
		title: verb + " " + n.Name + " to",
		items: labels,
		pick: func(m *Model, i int) tea.Cmd {
			d := dests[i]
			var into *config.Folder
			where := "the top level"
			if d != nil {
				into, where = d.Folder, d.Name
				m.expanded[d.ID] = true
			}
			st := &m.cfg.Structure
			var match func(*tree.Node) bool
			switch {
			case n.Folder != nil:
				f := n.Folder
				if move {
					st.RemoveFolder(n.ParentFolder(), f)
				} else {
					f = f.Clone()
				}
				if into != nil {
					f.Catalog = "" // only top-level categories stand for a catalog topic
				}
				st.AddFolder(into, f)
				match = func(t *tree.Node) bool { return t.Folder == f }
			case n.Entry != nil:
				e := n.Entry
				if move {
					st.RemoveEntry(n.ParentFolder(), e)
				} else {
					e = e.Clone()
					e.Scanned = false
				}
				st.AddEntry(into, e)
				match = func(t *tree.Node) bool { return t.Entry == e }
			default:
				e := &config.Entry{Name: n.Name, Desc: n.Desc, Path: fsinfo.Abbrev(n.FullPath()),
					Kind: n.Kind, Hint: n.Hint, Defaults: n.Defaults}
				st.AddEntry(into, e)
				match = func(t *tree.Node) bool { return t.Entry == e }
			}
			done := "Copied "
			if move {
				done = "Moved "
			}
			m.saveConfig(done + n.Name + " to " + where)
			m.selectWhere(match)
			return nil
		},
	})
}

func (m *Model) remove(n *tree.Node) {
	st := &m.cfg.Structure
	switch {
	case n == nil:
	case n.ID == tree.RecentID:
		m.openDialog(&confirm{title: "Clear the Recent list?", yes: func(m *Model) tea.Cmd {
			m.recent.Clear()
			m.saveRecent("Recent list cleared")
			return nil
		}})
	case n.Origin == tree.Recent:
		m.recent.Remove(n.Path)
		m.saveRecent("Removed " + n.Name + " from Recent")
	case n.Origin == tree.Dynamic:
		m.fail("This is a file on disk; confnav never deletes files. Remove the entry above it instead")
	case n.Folder != nil:
		f := n.Folder
		body := "Only the entry is removed; the file on disk is not touched."
		if count := f.Count(); count > 0 || len(f.Folders) > 0 {
			body = fmt.Sprintf("The folder and its %d entries are removed from confnav. Files on disk are not touched.", count)
		}
		if n.Parent == nil && f.Catalog != "" && m.opt.Catalog.Category(f.Catalog) != nil {
			body += " You can add the built-in category back in preferences (" + m.keys.label("preferences") + ")."
		}
		m.openDialog(&confirm{title: "Remove " + n.Name + "?", body: body, yes: func(m *Model) tea.Cmd {
			st.AddIgnore(f.ScannedPaths()...)
			st.RemoveFolder(n.ParentFolder(), f)
			m.saveConfig("Removed " + n.Name)
			return nil
		}})
	default:
		e := n.Entry
		body := "Only the entry is removed; the file on disk is not touched."
		if e.Scanned {
			body += " A new scan won't add it again."
		}
		m.openDialog(&confirm{title: "Remove " + n.Name + "?", body: body, yes: func(m *Model) tea.Cmd {
			if e.Scanned {
				st.AddIgnore(e.Path)
			}
			st.RemoveEntry(n.ParentFolder(), e)
			m.saveConfig("Removed " + n.Name)
			return nil
		}})
	}
}

// yank copies the full path to the clipboard with an OSC 52 escape
// sequence, which works over SSH and needs no clipboard tool.
func (m *Model) yank(n *tree.Node) {
	if n == nil || n.Kind == kind.Folder {
		m.fail("Select an entry to copy its path")
		return
	}
	seq := osc52.New(n.FullPath())
	switch {
	case os.Getenv("TMUX") != "":
		seq = seq.Tmux()
	case strings.HasPrefix(os.Getenv("TERM"), "screen"):
		seq = seq.Screen()
	}
	// stderr is the same terminal but not the renderer's stream.
	_, _ = seq.WriteTo(os.Stderr)
	m.setStatus(sOK, "Copied "+n.FullPath())
}

// saveConfig writes config.toml after a change to the list or settings.
func (m *Model) saveConfig(okMsg string) {
	err := config.Save(m.opt.ConfigPath, m.cfg)
	m.rebuild()
	if err != nil {
		m.fail("Could not save config: " + err.Error())
		return
	}
	m.setStatus(sOK, okMsg)
}

func (m *Model) saveRecent(okMsg string) {
	err := m.recent.Save()
	m.rebuild()
	if err != nil {
		m.fail("Could not save recent list: " + err.Error())
		return
	}
	m.setStatus(sOK, okMsg)
}
