package ui

import "testing"

func TestLayoutWideFitsEverything(t *testing.T) {
	c := layout(160, 20, 40, 30)
	if c.name != 20 || c.desc != 40 || c.mod != modWidth {
		t.Errorf("layout = %+v", c)
	}
	if total := c.name + c.desc + c.path + c.mod + 3*gap; total != 160 {
		t.Errorf("row does not use the full width: %d", total)
	}
}

func TestLayoutShrinks(t *testing.T) {
	c := layout(100, 30, 60, 50)
	if c.desc == 0 || c.path < c.desc {
		t.Errorf("expected path to keep the larger share: %+v", c)
	}
	if total := c.name + c.desc + c.path + c.mod + 3*gap; total != 100 {
		t.Errorf("total width %d != 100 (%+v)", total, c)
	}
}

func TestLayoutNarrowHidesDescriptionThenModified(t *testing.T) {
	c := layout(70, 20, 60, 40)
	if c.desc != 0 || c.mod == 0 {
		t.Errorf("at 70 columns description should be hidden first: %+v", c)
	}
	if total := c.name + c.path + c.mod + 2*gap; total != 70 {
		t.Errorf("total width %d != 70 (%+v)", total, c)
	}
	c = layout(40, 20, 60, 40)
	if c.mod != 0 {
		t.Errorf("at 40 columns modified should be hidden: %+v", c)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncMiddle("/etc/NetworkManager/system-connections", 24); width(got) != 24 || got[len(got)-11:] != "connections" {
		t.Errorf("truncMiddle = %q", got)
	}
	if got := truncMiddle("/etc/hosts", 20); got != "/etc/hosts" {
		t.Errorf("short path changed: %q", got)
	}
	if got := truncEnd("description that is long", 10); got != "descripti…" {
		t.Errorf("truncEnd = %q", got)
	}
	if got := pad("ab", 5); got != "ab   " {
		t.Errorf("pad = %q", got)
	}
}
