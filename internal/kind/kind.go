// Package kind defines the types of entries confnav shows.
package kind

import "github.com/fath-kip/confnav/internal/fsinfo"

// Kind is the type of a tree entry.
type Kind string

const (
	Folder Kind = "folder" // a category or user folder; lives only in confnav
	File   Kind = "file"   // a single file, opened in the editor
	Dir    Kind = "dir"    // a real directory, expands to its contents
	Glob   Kind = "glob"   // a pattern, expands to the files it matches
)

// Valid reports whether k is a known entry kind.
func (k Kind) Valid() bool {
	switch k {
	case Folder, File, Dir, Glob:
		return true
	}
	return false
}

// Infer guesses the kind of an expanded path a user typed.
func Infer(path string) Kind {
	switch {
	case fsinfo.HasGlob(path):
		return Glob
	case fsinfo.Stat(path).IsDir:
		return Dir
	}
	return File
}
