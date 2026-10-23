// Command confnav is a terminal navigator for configuration files.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"text/tabwriter"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/fath-kip/confnav/internal/catalog"
	"github.com/fath-kip/confnav/internal/config"
	"github.com/fath-kip/confnav/internal/distro"
	"github.com/fath-kip/confnav/internal/fsinfo"
	"github.com/fath-kip/confnav/internal/kind"
	"github.com/fath-kip/confnav/internal/library"
	"github.com/fath-kip/confnav/internal/recent"
	"github.com/fath-kip/confnav/internal/tree"
	"github.com/fath-kip/confnav/internal/ui"
)

// version is set at build time by GoReleaser.
var version = "dev"

const usage = `confnav - find and edit configuration files

Usage:
  confnav [flags] [search terms]

Search terms start the app in search mode, e.g. "confnav dns".

Flags:
  --distro NAME   use the catalog for arch, debian or fedora instead of the detected one
  --list          print the catalog entries for this system and exit
  --version       print the version and exit
  --help          show this help

Files:
  %s
  %s
  %s
`

func main() {
	flags := flag.NewFlagSet("confnav", flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, usage, config.Path(), library.Path(), recent.Path())
	}
	distroFlag := flags.String("distro", "", "")
	list := flags.Bool("list", false, "")
	showVersion := flags.Bool("version", false, "")
	if err := flags.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			return
		}
		os.Exit(2)
	}
	if *showVersion {
		fmt.Println("confnav", buildVersion())
		return
	}
	if err := run(*distroFlag, *list, strings.Join(flags.Args(), " ")); err != nil {
		fmt.Fprintln(os.Stderr, "confnav:", err)
		os.Exit(1)
	}
}

func run(distroName string, list bool, query string) error {
	cat, err := catalog.Load()
	if err != nil {
		return err
	}
	info := distro.Detect()
	family := info.Family
	if distroName != "" {
		f, ok := distro.ParseFamily(distroName)
		if !ok {
			return fmt.Errorf("unknown distro %q (use arch, debian or fedora)", distroName)
		}
		family = f
	}

	cfgPath := config.Path()
	cfg, cfgExists, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("%s: %w", cfgPath, err)
	}
	lib, err := library.Load(library.Path())
	if err != nil {
		return fmt.Errorf("could not read your library (fix or move the file): %w", err)
	}
	rec := recent.Load(recent.Path(), cfg.RecentLimit)

	if list {
		roots, _ := tree.Build(tree.Source{Catalog: cat, Family: family, Library: lib, Recent: rec})
		printList(os.Stdout, roots)
		return nil
	}

	if os.Getenv("NO_COLOR") != "" {
		lipgloss.SetColorProfile(termenv.Ascii)
	}
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if cfg.Mouse {
		opts = append(opts, tea.WithMouseCellMotion())
	}
	model := ui.New(ui.Options{
		Catalog: cat, Distro: info, Family: family,
		Config: cfg, ConfigPath: cfgPath, FirstRun: !cfgExists,
		Library: lib, Recent: rec, Query: query,
	})
	_, err = tea.NewProgram(model, opts...).Run()
	return err
}

// printList writes every entry with its state, for scripts and quick checks.
func printList(w io.Writer, roots []*tree.Node) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "CATEGORY\tNAME\tKIND\tSTATE\tPATH")
	for _, n := range tree.Entries(roots) {
		state := "present"
		switch st := fsinfo.Stat(n.FullPath()).State; {
		case n.Kind == kind.Glob:
			tree.Load(n)
			state = fmt.Sprintf("%d match", len(n.Children))
		case st == fsinfo.Missing:
			state = "missing"
		case st == fsinfo.NoAccess:
			state = "no access"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", n.Crumb(), n.Name, n.Kind, state, n.FullPath())
	}
	tw.Flush()
}

func buildVersion() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return version
}
