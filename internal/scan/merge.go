package scan

import (
	"path/filepath"

	"github.com/fath-kip/confnav/internal/config"
	"github.com/fath-kip/confnav/internal/fsinfo"
	"github.com/fath-kip/confnav/internal/kind"
)

// Compact is how many new files of one directory are listed one by one;
// with more, the directory itself is added and expands to its files.
const Compact = 12

// Result says what Merge did.
type Result struct {
	Added   int // new entries in the structure
	Known   int // already listed, or removed by the user before
	Skipped int // their category is not in the list
}

// Merge adds found files that are not listed yet to their categories in s,
// marked as found by a scan. Files of categories the user removed are
// skipped; Applications and Other are created when needed.
func Merge(s *config.Structure, found []Found) Result {
	var r Result
	index := s.Index()
	var fresh []Found
	perGroup := map[string]int{}
	for _, f := range found {
		switch {
		case index.Covers(f.Path) || s.Ignored(f.Path):
			r.Known++
		case s.Category(f.Category) == nil && f.Category != Apps && f.Category != Other:
			r.Skipped++
		default:
			fresh = append(fresh, f)
			if f.Group != "" {
				perGroup[f.Group]++
			}
		}
	}

	done := map[string]bool{}
	for _, f := range fresh {
		e := &config.Entry{Name: filepath.Base(f.Path), Path: fsinfo.Abbrev(f.Path), Kind: kind.File, Scanned: true}
		if f.Group != "" && perGroup[f.Group] > Compact {
			if done[f.Group] {
				continue
			}
			done[f.Group] = true
			e.Name, e.Path, e.Kind = filepath.Base(f.Group), fsinfo.Abbrev(f.Group), kind.Dir
			if filepath.Base(f.Group) == f.Folder {
				f.Folder = "" // the directory is the folder's whole content
			}
		}
		s.AddEntry(place(s, f), e)
		r.Added++
	}
	return r
}

// place returns the folder a found file goes into, creating it if needed.
func place(s *config.Structure, f Found) *config.Folder {
	cat := s.Category(f.Category)
	if cat == nil {
		cat = &config.Folder{Catalog: f.Category, Name: Names[f.Category]}
		s.AddFolder(nil, cat)
	}
	if f.Folder == "" {
		return cat
	}
	for _, sub := range cat.Folders {
		if sub.Name == f.Folder {
			return sub
		}
	}
	sub := &config.Folder{Name: f.Folder, Scanned: true}
	s.AddFolder(cat, sub)
	return sub
}
