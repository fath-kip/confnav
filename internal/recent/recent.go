// Package recent keeps the list of recently opened files.
package recent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/fath-kip/confnav/internal/xdg"
)

// Item is one opened file.
type Item struct {
	Path   string    `json:"path"`
	Opened time.Time `json:"opened"`
}

// List is the recent list, newest first.
type List struct {
	Items []Item
	Limit int
	path  string
}

// Path returns the location of recent.json.
func Path() string { return filepath.Join(xdg.StateDir(), "recent.json") }

// Load reads the list. A missing or unreadable file starts an empty list:
// recent history is not worth refusing to start over.
func Load(path string, limit int) *List {
	l := &List{Limit: limit, path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		return l
	}
	if json.Unmarshal(data, &l.Items) != nil {
		l.Items = nil
	}
	l.trim()
	return l
}

// Add moves path to the top of the list.
func (l *List) Add(path string, at time.Time) {
	l.Remove(path)
	l.Items = append([]Item{{Path: path, Opened: at}}, l.Items...)
	l.trim()
}

// Remove drops path from the list.
func (l *List) Remove(path string) {
	out := l.Items[:0]
	for _, it := range l.Items {
		if it.Path != path {
			out = append(out, it)
		}
	}
	l.Items = out
}

// Clear empties the list.
func (l *List) Clear() { l.Items = nil }

// Save writes the list atomically.
func (l *List) Save() error {
	data, err := json.MarshalIndent(l.Items, "", "  ")
	if err != nil {
		return err
	}
	return xdg.WriteFileAtomic(l.path, append(data, '\n'), 0o644)
}

func (l *List) trim() {
	if l.Limit > 0 && len(l.Items) > l.Limit {
		l.Items = l.Items[:l.Limit]
	}
}
