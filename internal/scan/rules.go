package scan

import "strings"

// rules sort found files into catalog categories. A pattern is a location
// ("etc", "boot", "home") and a path relative to it; it matches that path
// and everything below it, or every path starting with it when it ends in
// "*". The first match wins, so specific rules come first.
var rules = []struct{ pattern, category string }{
	// Before the general systemd rule.
	{"etc:systemd/resolved.conf*", "dns"},
	{"etc:systemd/timesyncd.conf*", "locale"},
	{"etc:systemd/network", "network"},
	{"etc:systemd", "systemd"},
	{"home:.config/systemd", "systemd"},
	{"home:.config/environment.d", "shell"},

	{"etc:resolv.conf", "dns"}, {"etc:hosts", "dns"}, {"etc:host.conf", "dns"},
	{"etc:nsswitch.conf", "dns"}, {"etc:dnsmasq*", "dns"}, {"etc:unbound", "dns"},
	{"etc:avahi", "dns"}, {"etc:mdns*", "dns"},

	{"etc:NetworkManager", "network"}, {"etc:netplan", "network"}, {"etc:network", "network"},
	{"etc:iwd", "network"}, {"etc:wpa_supplicant", "network"}, {"etc:hostname", "network"},
	{"etc:dhcpcd.conf", "network"}, {"etc:dhcp", "network"}, {"etc:wireguard", "network"},
	{"etc:netctl", "network"}, {"etc:ppp", "network"}, {"etc:sysconfig/network*", "network"},
	{"etc:connman", "network"}, {"etc:openvpn", "network"},

	{"etc:default/grub", "boot"}, {"etc:grub.d", "boot"}, {"etc:mkinitcpio*", "boot"},
	{"etc:dracut*", "boot"}, {"etc:kernel", "boot"}, {"etc:initramfs-tools", "boot"},
	{"etc:limine*", "boot"}, {"etc:default/limine", "boot"}, {"etc:sdboot*", "boot"},
	{"etc:plymouth", "boot"}, {"boot:", "boot"},

	{"etc:pacman*", "packages"}, {"etc:makepkg*", "packages"}, {"etc:paru.conf", "packages"},
	{"etc:pamac*", "packages"}, {"etc:apt", "packages"}, {"etc:dnf", "packages"},
	{"etc:yum*", "packages"}, {"etc:flatpak", "packages"}, {"etc:reflector", "packages"},
	{"etc:xdg/reflector", "packages"}, {"home:.config/paru", "packages"}, {"home:.config/yay", "packages"},

	{"etc:fstab", "filesystems"}, {"etc:crypttab", "filesystems"}, {"etc:mdadm*", "filesystems"},
	{"etc:lvm", "filesystems"}, {"etc:btrfs*", "filesystems"}, {"etc:snapper", "filesystems"},
	{"etc:exports", "filesystems"}, {"etc:auto.*", "filesystems"}, {"etc:udisks2", "filesystems"},

	{"etc:sysctl*", "kernel"}, {"etc:modprobe.d", "kernel"}, {"etc:modules-load.d", "kernel"},
	{"etc:modules", "kernel"}, {"etc:udev", "kernel"}, {"etc:tlp*", "kernel"},

	{"etc:pam.d", "auth"}, {"etc:sudoers*", "auth"}, {"etc:sudo.conf", "auth"},
	{"etc:security", "auth"}, {"etc:polkit-1", "auth"}, {"etc:passwd", "auth"}, {"etc:group", "auth"},
	{"etc:shadow", "auth"}, {"etc:gshadow", "auth"}, {"etc:login.defs", "auth"},
	{"etc:doas.conf", "auth"}, {"etc:subuid", "auth"}, {"etc:subgid", "auth"}, {"etc:shells", "auth"},

	{"etc:ssh", "ssh"}, {"home:.ssh", "ssh"},

	{"etc:ufw", "firewall"}, {"etc:firewalld", "firewall"}, {"etc:nftables*", "firewall"},
	{"etc:iptables", "firewall"}, {"etc:fail2ban", "firewall"},

	{"etc:X11/xorg.conf.d/00-keyboard.conf", "locale"},
	{"etc:locale*", "locale"}, {"etc:vconsole.conf", "locale"}, {"etc:localtime", "locale"},
	{"etc:timezone", "locale"}, {"etc:chrony*", "locale"}, {"etc:ntp*", "locale"}, {"etc:adjtime", "locale"},

	{"etc:profile*", "shell"}, {"etc:bash*", "shell"}, {"etc:zsh*", "shell"}, {"etc:environment", "shell"},
	{"etc:fish", "shell"}, {"etc:inputrc", "shell"},
	{"home:.bash*", "shell"}, {"home:.zsh*", "shell"}, {"home:.zprofile", "shell"}, {"home:.zlogin", "shell"},
	{"home:.profile", "shell"}, {"home:.inputrc", "shell"}, {"home:.tmux.conf", "shell"},
	{"home:.config/fish", "shell"}, {"home:.config/zsh", "shell"}, {"home:.config/tmux", "shell"},
	{"home:.config/starship.toml", "shell"},

	{"etc:pipewire", "audio"}, {"etc:pulse", "audio"}, {"etc:wireplumber", "audio"},
	{"etc:asound.conf", "audio"}, {"etc:alsa", "audio"}, {"home:.asoundrc", "audio"},
	{"home:.config/pipewire", "audio"}, {"home:.config/pulse", "audio"}, {"home:.config/wireplumber", "audio"},

	{"etc:X11", "desktop"}, {"etc:xdg", "desktop"}, {"etc:sddm*", "desktop"}, {"etc:gdm*", "desktop"},
	{"etc:lightdm", "desktop"}, {"etc:greetd", "desktop"},
	{"home:.Xresources", "desktop"}, {"home:.Xdefaults", "desktop"}, {"home:.xinitrc", "desktop"},
	{"home:.xprofile", "desktop"}, {"home:.xsession", "desktop"}, {"home:.local/share/applications", "desktop"},
	{"home:.config/hypr", "desktop"}, {"home:.config/sway", "desktop"}, {"home:.config/i3", "desktop"},
	{"home:.config/niri", "desktop"}, {"home:.config/river", "desktop"}, {"home:.config/labwc", "desktop"},
	{"home:.config/kde*", "desktop"}, {"home:.config/kwin*", "desktop"}, {"home:.config/plasma*", "desktop"},
	{"home:.config/kglobalshortcutsrc", "desktop"}, {"home:.config/gtk-3.0", "desktop"},
	{"home:.config/gtk-4.0", "desktop"}, {"home:.config/waybar", "desktop"}, {"home:.config/rofi", "desktop"},
	{"home:.config/wofi", "desktop"}, {"home:.config/dunst", "desktop"}, {"home:.config/mako", "desktop"},
	{"home:.config/swaync", "desktop"}, {"home:.config/picom*", "desktop"}, {"home:.config/xfce4", "desktop"},
	{"home:.config/openbox", "desktop"}, {"home:.config/awesome", "desktop"}, {"home:.config/bspwm", "desktop"},
	{"home:.config/sxhkd", "desktop"}, {"home:.config/polybar", "desktop"}, {"home:.config/eww", "desktop"},
	{"home:.config/fontconfig", "desktop"}, {"home:.config/autostart", "desktop"},
	{"home:.config/mimeapps.list", "desktop"}, {"home:.config/user-dirs.*", "desktop"},
	{"home:.config/qt5ct", "desktop"}, {"home:.config/qt6ct", "desktop"},
}

// classify finds the category of a path relative to its location.
func classify(base, rel string) Found {
	for _, r := range rules {
		loc, p, _ := strings.Cut(r.pattern, ":")
		if loc == base && matches(p, rel) {
			return Found{Category: r.category}
		}
	}
	parts := strings.Split(rel, "/")
	switch {
	case base == "home" && parts[0] == ".config" && len(parts) > 2:
		return Found{Category: Apps, Folder: parts[1]}
	case base == "home":
		return Found{Category: Apps}
	case len(parts) > 1:
		return Found{Category: Other, Folder: parts[0]}
	}
	return Found{Category: Other}
}

func matches(pattern, rel string) bool {
	if pattern == "" {
		return true
	}
	if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
		return strings.HasPrefix(rel, prefix)
	}
	return rel == pattern || strings.HasPrefix(rel, pattern+"/")
}
