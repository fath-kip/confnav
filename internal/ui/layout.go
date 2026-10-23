package ui

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

const (
	gap      = 2  // spaces between columns
	modWidth = 16 // "2006-01-02 15:04"
)

// columns holds the width of each column; 0 means the column is hidden.
type columns struct {
	name, desc, path, mod int
}

// layout splits the terminal width between the columns. n* are the widest
// values of each column, so a wide terminal shows everything untruncated
// and the path column absorbs the slack. When space runs out the name is
// capped first, then path and description share what is left, and the
// description is dropped before anything becomes unreadable.
func layout(width, nName, nDesc, nPath int) columns {
	c := columns{mod: modWidth}
	avail := width - c.mod - 3*gap
	if avail < 36 {
		c.mod = 0
		avail = width - 2*gap
	}
	c.name = max(1, min(nName, max(12, avail*3/10)))
	rest := avail - c.name
	if nDesc+nPath <= rest {
		c.desc = nDesc
		c.path = rest - nDesc
		return c
	}
	c.path = min(nPath, max(rest*55/100, rest-nDesc))
	c.desc = rest - c.path
	// The path is what people come here for; give up the description
	// before the path gets too short to recognise.
	if c.desc < 12 || c.path < min(nPath, 28) {
		c.desc = 0
		c.path = rest + gap
	}
	c.path = max(1, c.path)
	return c
}

// join lays out the visible cells with gaps between them.
func (c columns) join(name, desc, path, mod string) string {
	parts := []string{name}
	if c.desc > 0 {
		parts = append(parts, desc)
	}
	parts = append(parts, path)
	if c.mod > 0 {
		parts = append(parts, mod)
	}
	return strings.Join(parts, strings.Repeat(" ", gap))
}

func width(s string) int { return runewidth.StringWidth(s) }

// pad truncates s at the end and fills it to exactly w cells.
func pad(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return runewidth.FillRight(truncEnd(s, w), w)
}

func truncEnd(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if width(s) <= w {
		return s
	}
	return runewidth.Truncate(s, w, "…")
}

// truncMiddle keeps the start and, preferably, the end of s, so a long path
// still shows its file name: /etc/NetworkMa…/system-connections.
func truncMiddle(s string, w int) string {
	if width(s) <= w {
		return s
	}
	if w < 6 {
		return truncEnd(s, w)
	}
	tail := (w - 1) * 3 / 5
	head := w - 1 - tail
	r := []rune(s)
	var b strings.Builder
	for i, used := 0, 0; i < len(r); i++ {
		rw := runewidth.RuneWidth(r[i])
		if used+rw > head {
			break
		}
		b.WriteRune(r[i])
		used += rw
	}
	b.WriteString("…")
	var end []rune
	for i, used := len(r)-1, 0; i >= 0; i-- {
		rw := runewidth.RuneWidth(r[i])
		if used+rw > tail {
			break
		}
		end = append([]rune{r[i]}, end...)
		used += rw
	}
	b.WriteString(string(end))
	return b.String()
}
