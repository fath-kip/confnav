package ui

import "github.com/charmbracelet/lipgloss"

// Colours use the terminal's own 16-colour palette so they follow its theme.
var (
	sPlain      = lipgloss.NewStyle()
	sBold       = lipgloss.NewStyle().Bold(true)
	sFaint      = lipgloss.NewStyle().Faint(true)
	sHeader     = lipgloss.NewStyle().Faint(true).Bold(true)
	sFolder     = lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true)
	sDir        = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	sPath       = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	sSelected   = lipgloss.NewStyle().Reverse(true)
	sError      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	sOK         = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	sHint       = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	sModal      = lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(lipgloss.Color("4")).Padding(1, 2)
	sModalTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("4")).Padding(0, 1)
	sPicked     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("4"))
)
