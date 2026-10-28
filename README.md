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
+
────────────────────────────────────────────────────────────────────────────────────────────────────
/etc/systemd/resolved.conf · after edit: sudo systemctl restart systemd-resolved
enter open · a add · r edit · d remove · / search · shift+s settings · shift+p prefs · shift+m hide missing · ? help · q quit
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
- **Guided first start**: confnav asks for your editor, which categories you want, whether to show
  files you don't have and whether to scan the system. `shift+p` brings these preferences back.
- **Finds what the catalog doesn't know**: a scan (`shift+f`) looks through `/etc`, the boot loader,
  `~/.config` and your dotfiles and adds the configuration files it finds to the matching categories;
  the rest goes to *Applications* (per app) and *Other*. As root (through `sudo`) it also sees
  directories only root can read. Backups, keys, certificates, caches and binary files are skipped.
- **Your own folders**: add folders, files, directories or globs anywhere, including the top level
  (the `+` row at the bottom). Rename, copy, move or remove anything in the list.
  confnav never deletes or changes files on disk.
- **Everything is in one file**: categories and their files live in `config.toml` next to the
  settings, with a comment on every line, so you can edit the list there too.
- **Hide what you don't have**: `shift+m` hides every listed file that doesn't exist on your system
  and shows them again. Install a package later and its files simply appear.
- **Its own settings are one key away**: `shift+s` opens confnav's config file. Save it and the new
  settings, key bindings included, apply immediately.
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
confnav --list        print your list (category, name, kind, state, path)
confnav --scan        print the configuration files a scan finds (sudo confnav --scan for everything)
confnav --distro fedora   use another distribution's catalog for new categories
```

| Key | Action |
|---|---|
| `↑`/`k` `↓`/`j` | move |
| `←`/`h` `→`/`l` | collapse / expand, or go to parent |
| `enter`, `space` | open the file, or toggle the folder |
| `o` | open once with another editor |
| `shift+e` | choose the default editor |
| `shift+s` | open confnav's own settings (`config.toml`) |
| `shift+p` | preferences: editor, categories, missing files, scanning |
| `shift+f` | scan the system for configuration files that are not listed yet |
| `/` | search; `esc` shows the result in the tree |
| `a` | add a file, directory, glob or folder to the selected folder (`tab` completes paths) |
| `r` | rename or edit |
| `c` / `m` | copy / move to another folder |
| `d` | remove from the list (files on disk are never touched) |
| `shift+m` | hide / show files that don't exist on this system |
| `y` | copy the path to the clipboard (OSC 52, works over SSH) |
| `ctrl+r` | refresh and reload settings |
| `?` | help (scroll with `↑↓`, any other key closes it) |
| `q` | quit |

The `+` row at the bottom adds at the top level (enter or double-click). With the mouse, a click
toggles a folder and a double-click opens a file.

## Configuration

On first start confnav asks a few questions (editor, categories, missing files, scanning); `shift+p`
asks them again later. Everything is stored in `~/.config/confnav/config.toml`, which `shift+s` opens
in your editor. Saving it applies the changes immediately; a file with an error keeps the previous
settings and shows the error. Every setting and key has its explanation in a comment above it:

```toml
# Command that opens files, e.g. "micro", "nano", "vim", "nvim -p".
# Empty means: $VISUAL, then $EDITOR, then the first installed editor. (shift+e)
editor = "micro"

# How a scan (shift+f) runs: "user" sees what you can read, "root" uses sudo
# to also see protected directories. Empty: ask on the next scan.
scan_as = "user"

[keys]

# Rename or edit.
edit = ["r", "f2"]
```

The actions are `up`, `down`, `collapse`, `expand`, `top`, `bottom`, `page_up`, `page_down`, `open`,
`open_with`, `editor`, `settings`, `preferences`, `scan`, `add`, `edit`, `copy`, `move`, `remove`,
`toggle_missing`, `yank`, `refresh`, `search`, `help` and `quit`. Actions you leave out keep their
default keys; `ctrl+c` always quits. Unknown actions and keys bound twice are reported in the status line.

The list itself is under `[category_structure]`, in the order it is shown:

```toml
[category_structure]
ignore = []                              # scan results you removed; a new scan skips them

[[category_structure.category]]
catalog = "dns"                          # the built-in topic; leave it out for your own
name = "DNS"

  [[category_structure.category.entry]]
  name = "resolved.conf"
  desc = "systemd-resolved: DNS servers, DNS over TLS, DNSSEC"
  path = "/etc/systemd/resolved.conf"
  kind = "file"                          # or "dir", or "glob" for e.g. /etc/sysctl.d/*.conf
  hint = "sudo systemctl restart systemd-resolved"
  defaults = "/usr/lib/systemd/resolved.conf"

  [[category_structure.category.folder]] # folders nest: …folder.entry, …folder.folder
  name = "VPN"
```

confnav rewrites the file when you change something in the app, so comments you add yourself are not
kept. The recent list is in `~/.local/state/confnav/recent.json`. `NO_COLOR` is respected.

Version 0.1 kept your folders, hidden entries and renames in `~/.local/share/confnav/library.json`.
The first start of a newer version moves them into `config.toml` and keeps the old file as
`library.json.migrated`.

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

Without Go on the host, [`compose.yaml`](compose.yaml) runs the same steps in containers
(`podman-compose` or `docker compose`):

```
podman-compose run --rm test       # gofmt check, go vet, go test
podman-compose run --rm build      # ./confnav; VERSION=1.2.3 sets --version
podman-compose run --rm lint       # golangci-lint
podman-compose run --rm snapshot   # all release packages in ./dist
sudo install -Dm755 confnav /usr/local/bin/confnav
```

Releases are built by GitHub Actions when a `v*` tag is pushed.

## License

MIT
