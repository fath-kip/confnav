package catalog

import (
	"testing"

	"github.com/fath-kip/confnav/internal/distro"
	"github.com/fath-kip/confnav/internal/kind"
)

func TestBuiltinCatalog(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Categories) < 10 {
		t.Fatalf("only %d categories", len(c.Categories))
	}
	catIDs := map[string]bool{}
	for _, cat := range c.Categories {
		if catIDs[cat.ID] {
			t.Errorf("duplicate category id %q", cat.ID)
		}
		catIDs[cat.ID] = true
	}
	// An ID may repeat across families (same path, different hint) but must
	// be unique within what a single family sees.
	for _, f := range append([]distro.Family{distro.Unknown}, distro.Families...) {
		seen := map[string]bool{}
		count := 0
		for _, cat := range c.Categories {
			for _, e := range cat.Entries {
				if !e.For(f) {
					continue
				}
				count++
				id := EntryID(cat.ID, e.Path)
				if seen[id] {
					t.Errorf("family %q: duplicate entry %s", f, id)
				}
				seen[id] = true
			}
		}
		if f != distro.Unknown && count < 60 {
			t.Errorf("family %q sees only %d entries", f, count)
		}
	}
}

func TestParseRejectsBadEntries(t *testing.T) {
	bad := map[string]string{
		"unknown key":    "[[category]]\nid='a'\nname='A'\n[[category.entry]]\nname='x'\ndesc='d'\npath='/x'\ncolour='red'\n",
		"relative path":  "[[category]]\nid='a'\nname='A'\n[[category.entry]]\nname='x'\ndesc='d'\npath='etc/x'\n",
		"unknown distro": "[[category]]\nid='a'\nname='A'\n[[category.entry]]\nname='x'\ndesc='d'\npath='/x'\ndistros=['gentoo']\n",
		"glob mismatch":  "[[category]]\nid='a'\nname='A'\n[[category.entry]]\nname='x'\ndesc='d'\npath='/x/*.conf'\n",
		"missing desc":   "[[category]]\nid='a'\nname='A'\n[[category.entry]]\nname='x'\npath='/x'\n",
		"folder kind":    "[[category]]\nid='a'\nname='A'\n[[category.entry]]\nname='x'\ndesc='d'\npath='/x'\nkind='folder'\n",
	}
	for name, data := range bad {
		if _, err := Parse([]byte(data)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestEntryHelpers(t *testing.T) {
	e := Entry{Kind: kind.File, Distros: []distro.Family{distro.Arch}, Hint: "generic",
		Hints: map[string]string{"debian": "deb"}}
	if !e.For(distro.Arch) || e.For(distro.Fedora) || e.For(distro.Unknown) {
		t.Error("For() mismatch")
	}
	if e.HintFor(distro.Debian) != "deb" || e.HintFor(distro.Fedora) != "generic" {
		t.Error("HintFor() mismatch")
	}
	if !(Entry{}).For(distro.Unknown) {
		t.Error("entry without distros must apply to unknown distros")
	}
}
