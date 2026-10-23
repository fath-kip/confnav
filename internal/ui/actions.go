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
	"github.com/fath-kip/confnav/internal/library"
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

// chooseEditor sets the default editor. On first run, cancelling still
// writes the config so the question is not asked again.
func (m *Model) chooseEditor(firstRun bool) {
	current := editor.Resolve(m.cfg.Editor)
	items := editor.Detect()
	sel := 0
	for i, e := range items {
		if e == current {
			sel = i
		}
	}
	labels := make([]string, len(items))
	for i, e := range items {
		labels[i] = e
		if e == current {
			labels[i] += "  (current)"
		}
	}
	labels = append(labels, "Custom command…")
	title, subtitle := "Default editor", "Which editor should open your files?"
	if firstRun {
		title = "Welcome to confnav!"
		subtitle = "Which editor should open your files? You can change it later with " +
			m.keys.label("editor") + "."
	}
	set := func(m *Model, ed string) {
		m.cfg.Editor = ed
		if err := config.Save(m.opt.ConfigPath, m.cfg); err != nil {
			m.fail("Could not save config: " + err.Error())
			return
		}
		m.setStatus(sOK, "Files now open with "+ed+" (change with "+m.keys.label("editor")+")")
	}
	m.openDialog(&picker{
		title:    title,
		subtitle: subtitle,
		items:    labels,
		sel:      sel,
		pick: func(m *Model, i int) tea.Cmd {
			if i < len(items) {
				set(m, items[i])
				return nil
			}
			m.openDialog(newForm("Default editor", func(m *Model, v []string) (tea.Cmd, error) {
				if v[0] == "" {
					return nil, errors.New("enter a command")
				}
				set(m, v[0])
				return nil, nil
			}, fieldSpec{label: "Command", value: m.cfg.Editor, placeholder: "e.g. nvim -p"}))
			return nil
		},
		cancel: func(m *Model) {
			if !firstRun {
				return
			}
			if err := config.Save(m.opt.ConfigPath, m.cfg); err != nil {
				m.fail("Could not save config: " + err.Error())
			}
		},
	})
}

// ---- organising ----

// target says where new nodes go: a library container and its tree node
// (nil for the top level).
type target struct {
	container string
	folder    *tree.Node
}

func (t target) name() string {
	if t.folder == nil {
		return "the top level"
	}
	return t.folder.Name
}

// targetFor finds where new nodes go for the selected node: its folder,
// or the top level for top-level entries, Recent and the + row.
func targetFor(n *tree.Node) target {
	if n != nil {
		if f := n.Folder(); f != nil {
			if c, ok := f.Container(); ok {
				return target{c, f}
			}
		}
	}
	return target{container: library.Root}
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
		node := &library.Node{ID: library.NewID(), Kind: kind.Infer(full), Name: name, Desc: v[2], Path: p}
		if err := m.lib.Insert(t.container, node); err != nil {
			return nil, err
		}
		if t.folder != nil {
			m.expanded[t.folder.ID] = true
		}
		m.saveLibrary("Added " + name)
		m.selectID(node.ID)
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
		node := &library.Node{ID: library.NewID(), Kind: kind.Folder, Name: v[0]}
		if err := m.lib.Insert(t.container, node); err != nil {
			return nil, err
		}
		if t.folder != nil {
			m.expanded[t.folder.ID] = true
		}
		m.saveLibrary("Created folder " + v[0])
		m.selectID(node.ID)
		return nil, nil
	}, fieldSpec{label: "Name", placeholder: "e.g. Daily"}))
}

func (m *Model) editNode(n *tree.Node) {
	switch {
	case n == nil:
		return
	case n.Origin == tree.Recent || n.Origin == tree.Dynamic:
		m.fail("Only catalog entries and your own entries can be edited; copy this one to a folder with c")
		return
	case n.Origin == tree.Builtin:
		m.openDialog(newForm("Rename built-in "+n.Name, func(m *Model, v []string) (tea.Cmd, error) {
			m.lib.SetOverride(n.ID, v[0], v[1])
			m.saveLibrary("Updated " + n.Name)
			return nil, nil
		},
			fieldSpec{label: "Name", value: n.Name, placeholder: "empty = built-in name"},
			fieldSpec{label: "Description", value: n.Desc, placeholder: "empty = built-in description"},
		))
		return
	}
	u := m.lib.Find(n.ID)
	if u == nil {
		return
	}
	if u.Kind == kind.Folder {
		m.openDialog(newForm("Rename folder", func(m *Model, v []string) (tea.Cmd, error) {
			if v[0] == "" {
				return nil, errors.New("a name is required")
			}
			u.Name = v[0]
			m.saveLibrary("Renamed to " + v[0])
			return nil, nil
		}, fieldSpec{label: "Name", value: u.Name}))
		return
	}
	m.openDialog(newForm("Edit "+u.Name, func(m *Model, v []string) (tea.Cmd, error) {
		p := cleanPath(v[0])
		full := fsinfo.Expand(p)
		if p == "" || !filepath.IsAbs(full) {
			return nil, errors.New("the path must be absolute or start with ~/")
		}
		if v[1] == "" {
			v[1] = filepath.Base(full)
		}
		u.Path, u.Name, u.Desc, u.Kind = p, v[1], v[2], kind.Infer(full)
		m.saveLibrary("Updated " + u.Name)
		return nil, nil
	},
		fieldSpec{label: "Path", value: u.Path, path: true},
		fieldSpec{label: "Name", value: u.Name},
		fieldSpec{label: "Description", value: u.Desc},
	))
}

// copyOrMove puts n into another folder. Built-in items can't really move,
// so moving one copies it and hides the original.
func (m *Model) copyOrMove(n *tree.Node, move bool) {
	if n == nil || n.ID == tree.RecentID {
		m.fail("Select an entry or folder")
		return
	}
	verb := "Copy"
	if move {
		verb = "Move"
	}
	type dest struct {
		container string
		node      *tree.Node
	}
	var dests []dest
	var labels []string
	var walk func([]*tree.Node)
	walk = func(nodes []*tree.Node) {
		for _, f := range nodes {
			if f.Kind != kind.Folder || (move && f.ID == n.ID) {
				continue
			}
			if c, ok := f.Container(); ok && f != n.Parent {
				label := f.Name
				if crumb := f.Crumb(); crumb != "" {
					label = crumb + " › " + f.Name
				}
				dests = append(dests, dest{c, f})
				labels = append(labels, label)
			}
			walk(f.Children)
		}
	}
	walk(m.roots)
	if n.Parent != nil || n.Origin != tree.User {
		dests = append(dests, dest{container: library.Root})
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
			where := "the top level"
			if d.node != nil {
				where = d.node.Name
				m.expanded[d.node.ID] = true
			}
			if move && n.Origin == tree.User {
				if err := m.lib.Move(n.ID, d.container); err != nil {
					m.fail(err.Error())
					return nil
				}
				m.saveLibrary("Moved " + n.Name + " to " + where)
				return nil
			}
			c := m.toLibrary(n)
			if err := m.lib.Insert(d.container, c); err != nil {
				m.fail(err.Error())
				return nil
			}
			msg := "Copied " + n.Name + " to " + where
			if move && n.Origin == tree.Builtin {
				m.lib.Hide(n.ID)
				msg = "Moved " + n.Name + " to " + where + " (the built-in original is hidden; R restores it)"
			}
			m.saveLibrary(msg)
			m.selectID(c.ID)
			return nil
		},
	})
}

// toLibrary converts a tree node into a new library node.
func (m *Model) toLibrary(n *tree.Node) *library.Node {
	if n.Origin == tree.User {
		if u := m.lib.Find(n.ID); u != nil {
			return library.Clone(u)
		}
	}
	ln := &library.Node{ID: library.NewID(), Kind: n.Kind, Name: n.Name, Desc: n.Desc, Path: n.Path}
	if n.Kind == kind.Folder {
		for _, c := range n.Children {
			ln.Children = append(ln.Children, m.toLibrary(c))
		}
	}
	return ln
}

func (m *Model) remove(n *tree.Node) {
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
	case n.Origin == tree.Builtin:
		what := "entry"
		if n.Kind == kind.Folder {
			what = "category"
		}
		m.openDialog(&confirm{
			title: "Hide the built-in " + what + " " + n.Name + "?",
			body:  "Nothing on disk changes. Press R later to restore hidden built-ins.",
			yes: func(m *Model) tea.Cmd {
				m.lib.Hide(n.ID)
				m.saveLibrary("Hid " + n.Name)
				return nil
			},
		})
	default:
		body := "Only the entry is removed; the file on disk is not touched."
		if n.Kind == kind.Folder && len(n.Children) > 0 {
			body = fmt.Sprintf("The folder and its %d entries are removed from confnav. Files on disk are not touched.", len(n.Children))
		}
		m.openDialog(&confirm{title: "Remove " + n.Name + "?", body: body, yes: func(m *Model) tea.Cmd {
			m.lib.Remove(n.ID)
			m.saveLibrary("Removed " + n.Name)
			return nil
		}})
	}
}

func (m *Model) restoreHidden() {
	count := len(m.lib.Hidden)
	if count == 0 {
		m.setStatus(sFaint, "Nothing is hidden")
		return
	}
	m.openDialog(&confirm{
		title: fmt.Sprintf("Restore %d hidden built-in item(s)?", count),
		yes: func(m *Model) tea.Cmd {
			m.lib.RestoreHidden()
			m.saveLibrary(fmt.Sprintf("Restored %d item(s)", count))
			return nil
		},
	})
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

func (m *Model) saveLibrary(okMsg string) {
	err := m.lib.Save()
	m.rebuild()
	if err != nil {
		m.fail("Could not save library: " + err.Error())
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
