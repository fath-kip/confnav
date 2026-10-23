// Package editor decides how a file gets opened: directly in the user's
// editor, through sudoedit when it is not writable, or with the dedicated
// safe tools (visudo, vipw, vigr) for files that can lock you out.
package editor

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fath-kip/confnav/internal/fsinfo"
)

// Known editors, in the order they are suggested.
var Known = []string{"micro", "nano", "vim", "nvim", "hx", "emacs -nw", "vi"}

// Overridable for tests.
var (
	lookPath = exec.LookPath
	geteuid  = os.Geteuid
	writable = fsinfo.Writable
)

// Detect returns the known editors that are installed.
func Detect() []string {
	var out []string
	for _, e := range Known {
		if _, err := lookPath(strings.Fields(e)[0]); err == nil {
			out = append(out, e)
		}
	}
	return out
}

// Resolve picks the editor command: the configured one, then $VISUAL,
// $EDITOR, the first installed known editor, and finally vi.
func Resolve(configured string) string {
	for _, c := range []string{configured, os.Getenv("VISUAL"), os.Getenv("EDITOR")} {
		if strings.TrimSpace(c) != "" {
			return strings.TrimSpace(c)
		}
	}
	if d := Detect(); len(d) > 0 {
		return d[0]
	}
	return "vi"
}

// Mode tells how a file is opened.
type Mode int

const (
	Direct   Mode = iota // editor runs as the current user
	Sudoedit             // sudoedit copies the file, the editor runs as the user
	Special              // visudo, vipw or vigr through sudo
)

// Launch is a prepared editor invocation.
type Launch struct {
	Cmd  *exec.Cmd
	Mode Mode
	Tool string // the special tool's name, for Special
}

// Prepare builds the command that opens path (already expanded) with editor.
func Prepare(editor, path string) (*Launch, error) {
	args := strings.Fields(editor)
	if len(args) == 0 {
		return nil, errors.New("no editor configured")
	}
	root := geteuid() == 0

	if tool, toolArgs, ok := special(path); ok {
		// visudo, vipw and vigr pick the editor from these variables.
		env := []string{"SUDO_EDITOR=" + editor, "VISUAL=" + editor, "EDITOR=" + editor}
		var cmd *exec.Cmd
		if root {
			cmd = exec.Command(tool, toolArgs...)
			cmd.Env = append(os.Environ(), env...)
		} else {
			if _, err := lookPath("sudo"); err != nil {
				return nil, errors.New(tool + " needs sudo, which is not installed")
			}
			// env makes the variables survive sudo's env_reset.
			cmd = exec.Command("sudo", append(append([]string{"env"}, env...), append([]string{tool}, toolArgs...)...)...)
		}
		return &Launch{Cmd: cmd, Mode: Special, Tool: tool}, nil
	}

	if root || writable(path) {
		return &Launch{Cmd: exec.Command(args[0], append(args[1:], path)...), Mode: Direct}, nil
	}

	if _, err := lookPath("sudoedit"); err != nil {
		return nil, errors.New("file is not writable and sudoedit (sudo) is not installed")
	}
	// sudoedit refuses to follow symlinks, so hand it the real file.
	target := path
	if real, err := filepath.EvalSymlinks(path); err == nil {
		target = real
	}
	cmd := exec.Command("sudoedit", target)
	cmd.Env = append(os.Environ(), "SUDO_EDITOR="+editor)
	return &Launch{Cmd: cmd, Mode: Sudoedit}, nil
}

// Tool returns the safe editing tool used for path, or "" for a normal editor.
func Tool(path string) string {
	tool, _, _ := special(path)
	return tool
}

// special returns the safe editing tool for files where a syntax error can
// lock the user out of the system.
func special(path string) (tool string, args []string, ok bool) {
	switch {
	case path == "/etc/sudoers" || strings.HasPrefix(path, "/etc/sudoers.d/"):
		return "visudo", []string{"-f", path}, true
	case path == "/etc/passwd":
		return "vipw", nil, true
	case path == "/etc/shadow":
		return "vipw", []string{"-s"}, true
	case path == "/etc/group":
		return "vigr", nil, true
	case path == "/etc/gshadow":
		return "vigr", []string{"-s"}, true
	}
	return "", nil, false
}
