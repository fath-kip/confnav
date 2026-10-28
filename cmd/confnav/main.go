// Command confnav is a terminal navigator for configuration files.
package main

import (
	"encoding/json"
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
	"github.com/fath-kip/confnav/internal/scan"
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
  --list          print the entries of your list and exit
  --scan          print the configuration files a scan finds and exit
                  (run it with sudo to include what only root can read)
  --home DIR      home directory to scan (default: yours)
  --version       print the version and exit
  --help          show this help

Files:
  %s
  %s
`

func main() {
	flags := flag.NewFlagSet("confnav", flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, usage, config.Path(), recent.Path())
	}
	distroFlag := flags.String("distro", "", "")
	list := flags.Bool("list", false, "")
	scanFlag := flags.Bool("scan", false, "")
	scanJSON := flags.Bool("scan-json", false, "") // for the root scan the app starts through sudo
	home, _ := os.UserHomeDir()
	homeFlag := flags.String("home", home, "")
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
	if *scanFlag || *scanJSON {
		found := scan.Run(scan.System(*homeFlag))
		if *scanJSON {
			if err := json.NewEncoder(os.Stdout).Encode(found); err != nil {
				os.Exit(1)
			}
			return
		}
		printScan(os.Stdout, found)
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
	sys := distro.Detect()
	family := sys.Family
	if distroName != "" {
		f, ok := distro.ParseFamily(distroName)
		if !ok {
			return fmt.Errorf("unknown distro %q (use arch, debian or fedora)", distroName)
		}
		family = f
	}

	cfgPath := config.Path()
	cfg, info, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("%s: %w", cfgPath, err)
	}
	if !info.HasStructure {
		if err := setUpStructure(&cfg, info, cat, family, list); err != nil {
			return err
		}
	}
	rec := recent.Load(recent.Path(), cfg.RecentLimit)

	if list {
		roots, _ := tree.Build(tree.Source{Structure: &cfg.Structure, Recent: rec})
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
		Catalog: cat, Distro: sys, Family: family,
		Config: cfg, ConfigPath: cfgPath, FirstRun: !info.Exists,
		Recent: rec, Query: query, Version: buildVersion(),
	})
	_, err = tea.NewProgram(model, opts...).Run()
	return err
}

// setUpStructure fills in [category_structure] for a config that has none:
// from library.json if an older version left one, which is then kept as
// library.json.migrated, or with every category. On a first run the
// preferences ask instead, unless only listing.
func setUpStructure(cfg *config.Config, info config.Info, cat *catalog.Catalog, family distro.Family, list bool) error {
	lib, err := library.Load(library.Path())
	if err != nil {
		return fmt.Errorf("could not read your library (fix or move the file): %w", err)
	}
	switch {
	case lib != nil:
		cfg.Structure = library.Migrate(lib, cat, family)
	case info.Exists || list:
		cfg.Structure = cat.DefaultStructure(family)
	default:
		return nil // first run: the preferences fill it in
	}
	if !info.Exists && lib == nil {
		return nil // listing on a first run: show the defaults without saving
	}
	if err := config.Save(config.Path(), *cfg); err != nil {
		return fmt.Errorf("could not save %s: %w", config.Path(), err)
	}
	if lib != nil {
		if err := os.Rename(library.Path(), library.MigratedPath()); err != nil {
			return fmt.Errorf("moved your library into %s but could not rename the old file: %w", config.Path(), err)
		}
	}
	return nil
}

// printScan writes what a scan found, grouped as the app would add it.
func printScan(w io.Writer, found []scan.Found) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "CATEGORY\tFOLDER\tPATH")
	for _, f := range found {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", f.Category, f.Folder, f.Path)
	}
	tw.Flush()
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
