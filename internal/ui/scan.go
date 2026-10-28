package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fath-kip/confnav/internal/config"
	"github.com/fath-kip/confnav/internal/scan"
)

// sudoDoneMsg reports that `sudo -v` (asking for the password) finished.
type sudoDoneMsg struct{ err error }

// scanDoneMsg carries what a scan found.
type scanDoneMsg struct {
	found []scan.Found
	err   error
}

// startScan scans the system in the background, as the user or through
// sudo as configured; the first time it asks which.
func (m *Model) startScan() tea.Cmd {
	if m.scanning {
		m.setStatus(sHint, "A scan is already running")
		return nil
	}
	home, _ := os.UserHomeDir()
	switch m.cfg.ScanAs {
	case "":
		m.askScan()
		return nil
	case config.ScanRoot:
		// The password prompt needs the terminal; the scan itself then runs
		// in the background with the cached sudo credentials.
		m.scanning = true
		return tea.ExecProcess(exec.Command("sudo", "-v"), func(err error) tea.Msg { return sudoDoneMsg{err} })
	}
	m.scanning = true
	m.setStatus(sFaint, "Scanning…")
	run := m.runScan
	return func() tea.Msg { return scanDoneMsg{found: run(scan.System(home))} }
}

func (m *Model) afterSudo(msg sudoDoneMsg) tea.Cmd {
	if msg.err != nil {
		m.scanning = false
		m.fail("sudo failed, nothing was scanned: " + msg.err.Error())
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		m.scanning = false
		m.fail("Cannot find the confnav binary for the root scan: " + err.Error())
		return nil
	}
	home, _ := os.UserHomeDir()
	m.setStatus(sFaint, "Scanning as root…")
	return func() tea.Msg {
		out, err := exec.Command("sudo", "-n", exe, "--scan-json", "--home", home).Output()
		if err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) && len(ee.Stderr) > 0 {
				err = errors.New(strings.TrimSpace(string(ee.Stderr)))
			}
			return scanDoneMsg{err: err}
		}
		var found []scan.Found
		if err := json.Unmarshal(out, &found); err != nil {
			return scanDoneMsg{err: err}
		}
		return scanDoneMsg{found: found}
	}
}

// afterScan adds what the scan found to the list and saves it.
func (m *Model) afterScan(msg scanDoneMsg) {
	m.scanning = false
	if msg.err != nil {
		m.fail("Scan failed: " + msg.err.Error())
		return
	}
	r := scan.Merge(&m.cfg.Structure, msg.found)
	text := fmt.Sprintf("Scan added %d file(s) (%d already listed", r.Added, r.Known)
	if r.Skipped > 0 {
		text += fmt.Sprintf(", %d skipped: their category is not in your list", r.Skipped)
	}
	text += ")"
	if r.Added == 0 {
		m.rebuild()
		m.setStatus(sOK, text)
		return
	}
	m.saveConfig(text)
}

// askScan asks how to scan when the preferences never did.
func (m *Model) askScan() {
	items := make([]string, len(scanChoices))
	for i, c := range scanChoices {
		items[i] = c.label
	}
	m.openDialog(&picker{
		title:    "Scan for configuration files",
		subtitle: "How should confnav scan?",
		note:     scanNote + " The answer is saved as scan_as in config.toml.",
		items:    items,
		pick: func(m *Model, i int) tea.Cmd {
			m.cfg.ScanAs = scanChoices[i].as
			if err := config.Save(m.opt.ConfigPath, m.cfg); err != nil {
				m.fail("Could not save config: " + err.Error())
				return nil
			}
			return m.startScan()
		},
	})
}
