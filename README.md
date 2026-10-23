# confnav

A small terminal app for people who keep forgetting where configuration files live.

confnav shows the well-known config files of **Arch, Debian and Fedora** (and their derivatives
such as CachyOS, EndeavourOS, Manjaro, Ubuntu, Mint, Nobara) grouped by topic. Pick one and it opens
in your editor, through `sudoedit` when the file belongs to root.

```
confnav  CachyOS · arch                                                        / search · ? help
NAME                  DESCRIPTION                               PATH                          MODIFIED
▸ Recent (3)
▸ ★ My Configs (4)
▾ DNS (6)
    resolv.conf       Nameservers and search domains            /etc/resolv.conf              2026-10-20 14:32
    hosts             Static hostname to IP mappings            /etc/hosts                    2026-08-11 09:10
    resolved.conf     systemd-resolved: DNS servers, DNS ove…   /etc/systemd/resolved.conf    2026-09-02 09:10
  ▸ resolved.conf.d/  Drop-in overrides for systemd-resolved    /etc/systemd/resolved.conf.d  missing
▸ Systemd (10)
▸ Boot (8)
  confnav settings    Editor, key bindings, mouse, backups      ~/.config/confnav/config.toml 2026-10-23 19:24
+
────────────────────────────────────────────────────────────────────────────────────────────────────
/etc/systemd/resolved.conf · after edit: sudo systemctl restart systemd-resolved
enter open · a add · r edit · d remove · / search · shift+e editor · shift+m hide missing · ? help · q quit
```

## Features

- **Built-in catalog** of ~90 files and directories per distribution: DNS, network, systemd, boot,
  packages, filesystems, kernel, users & auth, SSH, firewall, locale, shell, desktop and audio.
  Only the entries for your distribution are shown, and files that don't exist on your system are greyed out.
- **Opens files the safe way.** Files you can't write go through `sudoedit`, so your editor runs
  as you rather than root. `sudoers`, `passwd` and `group` always open with `visudo`, `vipw` and `vigr`.
- **Starts from defaults.** When a file such as `/etc/systemd/journald.conf` doesn't exist yet but your
  distribution ships defaults (Fedora keeps them in `/usr/lib/systemd/`), confnav offers to copy them first.
- **After-edit hints** remind you of the next step, e.g. `sudo grub-mkconfig -o /boot/grub/grub.cfg`.
  confnav only shows them; it never runs commands for you.
- **Directories and globs**: entries like `/etc/sudoers.d` or `/etc/systemd/system/*.d/*.conf` expand
  to the files inside.
- **Your own folders**: add folders, files, directories or globs anywhere, including the top level
  (the `+` row at the bottom). Copy built-in entries into your folders, rename or hide built-ins you
  don't need. confnav never deletes or changes files on disk.
- **Hide what you don't have**: `shift+m` hides every built-in file that doesn't exist on your system
  and shows them again. Install a package later and its files simply appear.
- **Its own settings are in the list** ("confnav settings"). Save the file and the new settings,
  key bindings included, apply immediately.
- **Recent** list of the files you opened last, plus fuzzy **search** over everything.
- A single static binary of about 5 MB with no runtime dependencies. Works with the mouse too.

## Install

Download the package for your system from the
[latest release](https://github.com/fath-kip/confnav/releases/latest), then install it:

| System | Command |
|---|---|
| Arch, CachyOS, EndeavourOS, Manjaro | `sudo pacman -U confnav-*-x86_64.pkg.tar.zst` |
| Debian, Ubuntu, Mint | `sudo apt install ./confnav_*_amd64.deb` |
| Fedora, Nobara | `sudo dnf install ./confnav-*.x86_64.rpm` |
| Any Linux | `tar xzf confnav_*_linux_amd64.tar.gz && sudo install confnav /usr/local/bin/` |

ARM64 (`aarch64`) packages are published too. With Go installed you can also run
`go install github.com/fath-kip/confnav/cmd/confnav@latest`.

## Usage

```
confnav               start
confnav dns           start with a search for "dns"
confnav --list        print the catalog for this system (name, kind, state, path)
confnav --distro fedora   use another distribution's catalog
```

| Key | Action |
|---|---|
| `↑`/`k` `↓`/`j` | move |
| `←`/`h` `→`/`l` | collapse / expand, or go to parent |
| `enter`, `space` | open the file, or toggle the folder |
| `o` | open once with another editor |
| `shift+e` | choose the default editor |
| `/` | search; `esc` shows the result in the tree |
| `a` | add a file, directory, glob or folder to the selected folder (`tab` completes paths) |
| `r` | rename or edit |
| `c` / `m` | copy / move to another folder |
| `d` | remove from the list (built-ins are hidden, files on disk are never touched) |
| `shift+r` | restore hidden built-ins |
| `shift+m` | hide / show files that don't exist on this system |
| `y` | copy the path to the clipboard (OSC 52, works over SSH) |
| `ctrl+r` | refresh and reload settings |
| `?` | help |
| `q` | quit |

The `+` row at the bottom adds at the top level (enter or double-click). With the mouse, a click
toggles a folder and a double-click opens a file.

## Configuration

On first start confnav asks which editor to use. Settings live in `~/.config/confnav/config.toml`,
which is listed in the app as **confnav settings**; saving it applies the changes immediately
(a file with a syntax error keeps the previous settings and shows the error).

```toml
editor = "micro"                 # empty: $VISUAL, then $EDITOR, then the first installed editor
backup = false                   # copy files to ~/.local/state/confnav/backups before opening
mouse = true                     # mouse support (hold Shift to select text)
recent_limit = 20
date_format = "2006-01-02 15:04" # Go time layout
show_missing = true              # list built-in files that don't exist here (shift+m)

[keys]                           # every action, with one key or a list of keys
add = "a"
edit = ["r", "f2"]
editor = "shift+e"               # same as "E"
quit = "q"
```

The actions are `up`, `down`, `collapse`, `expand`, `top`, `bottom`, `page_up`, `page_down`, `open`,
`open_with`, `editor`, `add`, `edit`, `copy`, `move`, `remove`, `restore`, `toggle_missing`, `yank`,
`refresh`, `search`, `help` and `quit`. Actions you leave out keep their default keys; `ctrl+c` always
quits. Unknown actions and keys bound twice are reported in the status line.

Your folders, hidden entries and renames are stored in `~/.local/share/confnav/library.json` and
the recent list in `~/.local/state/confnav/recent.json`. `NO_COLOR` is respected.

## Contributing to the catalog

The catalog is [`internal/catalog/catalog.toml`](internal/catalog/catalog.toml). Each entry has a name,
a description, a path and optionally `distros`, `kind`, `hint`/`hints` and `defaults`; the file
header documents every field. `go test ./...` validates it.

## Building

```
go build ./cmd/confnav
go test ./...
goreleaser release --snapshot --clean   # build all release packages locally
```

Releases are built by GitHub Actions when a `v*` tag is pushed.

## License

MIT
