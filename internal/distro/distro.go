// Package distro detects which distribution family the system belongs to.
package distro

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// Family is a distribution family the built-in catalog knows about.
type Family string

const (
	Unknown Family = ""
	Arch    Family = "arch"
	Debian  Family = "debian"
	Fedora  Family = "fedora"
)

// Families lists every supported family.
var Families = []Family{Arch, Debian, Fedora}

// Info describes the running system.
type Info struct {
	ID     string // os-release ID, e.g. "cachyos"
	Name   string // PRETTY_NAME, e.g. "CachyOS"
	Family Family
}

// known maps os-release IDs (and ID_LIKE tokens) to a family.
var known = map[string]Family{
	"arch":   Arch,
	"debian": Debian,
	"ubuntu": Debian,
	"fedora": Fedora,
	"rhel":   Fedora,
	"centos": Fedora,
}

var osReleasePaths = []string{"/etc/os-release", "/usr/lib/os-release"}

// Detect reads os-release from the standard locations.
func Detect() Info {
	for _, p := range osReleasePaths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		defer f.Close()
		return Parse(f)
	}
	return Info{}
}

// Parse reads an os-release formatted stream.
func Parse(r io.Reader) Info {
	vals := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		vals[k] = unquote(strings.TrimSpace(v))
	}
	info := Info{ID: vals["ID"], Name: vals["PRETTY_NAME"]}
	if info.Name == "" {
		info.Name = vals["NAME"]
	}
	info.Family = FamilyOf(vals["ID"], strings.Fields(vals["ID_LIKE"]))
	return info
}

// FamilyOf picks the family from the ID first, then from ID_LIKE in order.
func FamilyOf(id string, like []string) Family {
	for _, c := range append([]string{id}, like...) {
		if f, ok := known[strings.ToLower(c)]; ok {
			return f
		}
	}
	return Unknown
}

// ParseFamily validates a family name given on the command line.
func ParseFamily(s string) (Family, bool) {
	for _, f := range Families {
		if string(f) == strings.ToLower(s) {
			return f, true
		}
	}
	return Unknown, false
}

func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		v = v[1 : len(v)-1]
	}
	r := strings.NewReplacer(`\"`, `"`, `\\`, `\`, `\$`, `$`, "\\`", "`")
	return r.Replace(v)
}
