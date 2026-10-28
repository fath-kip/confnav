package ui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fath-kip/confnav/internal/catalog"
	"github.com/fath-kip/confnav/internal/config"
)

// prefs collects the answers of the preferences dialogs. Nothing changes
// until the last one, so cancelling halfway leaves everything as it was.
type prefs struct {
	firstRun    bool
	editor      string
	cats        []string // catalog category IDs to have in the list
	showMissing bool
	scanAs      string // empty: don't scan now
}

const prefSteps = 4

func (p *prefs) title(step int) string {
	name := "Preferences"
	if p.firstRun {
		name = "Welcome to confnav!"
	}
	return fmt.Sprintf("%s · %d/%d", name, step, prefSteps)
}

// cancel ends the dialogs early. On the first run the answers so far and
// the defaults for the rest are saved, so the questions are not asked again.
func (p *prefs) cancel(m *Model) {
	if p.firstRun {
		m.applyPrefs(p)
		return
	}
	m.setStatus(sFaint, "Preferences unchanged")
}

// openPrefs walks through the preferences: editor, categories, missing
// files and scanning.
func (m *Model) openPrefs(firstRun bool) {
	p := &prefs{firstRun: firstRun, editor: m.cfg.Editor, showMissing: m.cfg.ShowMissing}
	for _, cat := range m.categories() {
		// A new list starts with everything; later the list says what is in.
		if len(m.cfg.Structure.Categories) == 0 || m.cfg.Structure.Category(cat.ID) != nil {
			p.cats = append(p.cats, cat.ID)
		}
	}
	m.editorPicker(p.title(1), "Which editor should open your files?",
		"You can change it later with "+m.keys.label("editor")+", in preferences ("+
			m.keys.label("preferences")+") or in config.toml.",
		func(m *Model, ed string) tea.Cmd {
			p.editor = ed
			m.prefsCategories(p)
			return nil
		}, p.cancel)
}

// categories are the catalog categories that have files for this system.
func (m *Model) categories() []*catalog.Category {
	var out []*catalog.Category
	for i := range m.opt.Catalog.Categories {
		if cat := &m.opt.Catalog.Categories[i]; cat.Count(m.opt.Family) > 0 {
			out = append(out, cat)
		}
	}
	return out
}

func (m *Model) prefsCategories(p *prefs) {
	cats := m.categories()
	items := make([]string, len(cats))
	checked := make([]bool, len(cats))
	for i, cat := range cats {
		items[i] = fmt.Sprintf("%s (%d files)", cat.Name, cat.Count(m.opt.Family))
		checked[i] = slices.Contains(p.cats, cat.ID)
	}
	m.openDialog(&checklist{
		title:    p.title(2),
		subtitle: "Which categories do you want in the list?",
		note: "After setup you can rename or remove these categories, add your own and change their files: " +
			"in the list (" + m.keys.label("add") + " add, " + m.keys.label("edit") + " rename, " +
			m.keys.label("remove") + " remove, " + m.keys.label("copy") + "/" + m.keys.label("move") +
			" copy/move) or under [category_structure] in config.toml. Come back here (" +
			m.keys.label("preferences") + ") to add built-in categories again.",
		items:   items,
		checked: checked,
		done: func(m *Model, checked []bool) tea.Cmd {
			p.cats = nil
			for i, ok := range checked {
				if ok {
					p.cats = append(p.cats, cats[i].ID)
				}
			}
			m.prefsMissing(p)
			return nil
		},
		cancel: p.cancel,
	})
}

func (m *Model) prefsMissing(p *prefs) {
	sel := 0
	if !p.showMissing {
		sel = 1
	}
	m.openDialog(&picker{
		title:    p.title(3),
		subtitle: "Show files that don't exist on this system?",
		note: "Missing files are well-known configuration files that are not on this system (yet), " +
			"for example because a package isn't installed or its built-in defaults are in use. " +
			"Shown greyed out, they let you create one when you need it; hidden, the list stays short. " +
			"Toggle any time with " + m.keys.label("toggle_missing") + ".",
		items: []string{"Show them, greyed out", "Hide them"},
		sel:   sel,
		pick: func(m *Model, i int) tea.Cmd {
			p.showMissing = i == 0
			m.prefsScan(p)
			return nil
		},
		cancel: p.cancel,
	})
}

// scanChoices are the ways a scan can run, as offered in the dialogs.
var scanChoices = []struct{ label, as string }{
	{"Scan as the current user", config.ScanUser},
	{"Scan as root (sudo asks for your password)", config.ScanRoot},
}

// scanNote explains the two ways of scanning.
const scanNote = "A scan looks through /etc, the boot loader, ~/.config and your dotfiles for configuration " +
	"files that are not listed yet and adds them to the matching categories (others go to " +
	"Applications and Other). As root it also sees directories only root can read, such as " +
	"/etc/NetworkManager/system-connections and /etc/sudoers.d. Opening files is not affected: " +
	"files you can't write always open through sudoedit."

func (m *Model) prefsScan(p *prefs) {
	items := []string{scanChoices[0].label, scanChoices[1].label, "Don't scan now"}
	sel := 0
	if !p.firstRun {
		sel = 2
	}
	m.openDialog(&picker{
		title:    p.title(4),
		subtitle: "Look for more configuration files on this system?",
		note:     scanNote + " Scan again any time with " + m.keys.label("scan") + ".",
		items:    items,
		sel:      sel,
		pick: func(m *Model, i int) tea.Cmd {
			if i < len(scanChoices) {
				p.scanAs = scanChoices[i].as
			}
			return m.applyPrefs(p)
		},
		cancel: p.cancel,
	})
}

// applyPrefs saves the answers. Categories that were ticked are added from
// the catalog; removing unticked ones needs a confirmation, except on the
// first run.
func (m *Model) applyPrefs(p *prefs) tea.Cmd {
	st := &m.cfg.Structure
	empty := len(st.Categories) == 0 && len(st.Entries) == 0
	var removed []*config.Folder
	for _, cat := range m.categories() {
		want, have := slices.Contains(p.cats, cat.ID), st.Category(cat.ID)
		switch {
		case want && have == nil:
			st.Categories = append(st.Categories, m.opt.Catalog.Seed(m.opt.Family, []string{cat.ID})...)
		case !want && have != nil:
			removed = append(removed, have)
		}
	}
	if empty {
		st.Categories = append([]*config.Folder{{Name: catalog.MyConfigs}}, st.Categories...)
	}

	apply := func(m *Model, drop bool) tea.Cmd {
		if drop {
			for _, f := range removed {
				st.AddIgnore(f.ScannedPaths()...)
				st.RemoveFolder(nil, f)
			}
		}
		m.cfg.Editor, m.cfg.ShowMissing = p.editor, p.showMissing
		if p.scanAs != "" {
			m.cfg.ScanAs = p.scanAs
		}
		m.saveConfig("Preferences saved")
		if p.scanAs != "" {
			return m.startScan()
		}
		return nil
	}
	if len(removed) == 0 || p.firstRun {
		return apply(m, true)
	}
	names := make([]string, len(removed))
	count := 0
	for i, f := range removed {
		names[i] = f.Name
		count += f.Count()
	}
	title := "Remove the category " + names[0] + "?"
	if len(removed) > 1 {
		title = fmt.Sprintf("Remove %d categories?", len(removed))
	}
	m.openDialog(&confirm{
		title: title,
		body: fmt.Sprintf("%s and their %d entries are removed from the list; files on disk are not touched. "+
			"n keeps them and saves the other preferences.", strings.Join(names, ", "), count),
		yes: func(m *Model) tea.Cmd { return apply(m, true) },
		no:  func(m *Model) tea.Cmd { return apply(m, false) },
	})
	return nil
}
