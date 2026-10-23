package distro

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name, osRelease string
		want            Info
	}{
		{"cachyos", "NAME=\"CachyOS Linux\"\nPRETTY_NAME=\"CachyOS\"\nID=cachyos\nID_LIKE=arch\n",
			Info{ID: "cachyos", Name: "CachyOS", Family: Arch}},
		{"arch", "NAME=\"Arch Linux\"\nID=arch\n",
			Info{ID: "arch", Name: "Arch Linux", Family: Arch}},
		{"ubuntu", "PRETTY_NAME=\"Ubuntu 24.04 LTS\"\nID=ubuntu\nID_LIKE=debian\n",
			Info{ID: "ubuntu", Name: "Ubuntu 24.04 LTS", Family: Debian}},
		{"mint", "PRETTY_NAME='Linux Mint 22'\nID=linuxmint\nID_LIKE=\"ubuntu debian\"\n",
			Info{ID: "linuxmint", Name: "Linux Mint 22", Family: Debian}},
		{"nobara", "# comment\nID=nobara\nID_LIKE=\"rhel centos fedora\"\nPRETTY_NAME=\"Nobara Linux 41\"\n",
			Info{ID: "nobara", Name: "Nobara Linux 41", Family: Fedora}},
		{"opensuse", "ID=\"opensuse-tumbleweed\"\nID_LIKE=\"opensuse suse\"\nNAME=openSUSE\n",
			Info{ID: "opensuse-tumbleweed", Name: "openSUSE", Family: Unknown}},
		{"empty", "", Info{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Parse(strings.NewReader(tt.osRelease)); got != tt.want {
				t.Errorf("Parse() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseFamily(t *testing.T) {
	if f, ok := ParseFamily("Fedora"); !ok || f != Fedora {
		t.Errorf("ParseFamily(Fedora) = %q, %v", f, ok)
	}
	if _, ok := ParseFamily("gentoo"); ok {
		t.Error("ParseFamily(gentoo) should fail")
	}
}
