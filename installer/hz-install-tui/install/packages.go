package install

// Kernels installed via pacstrap (matches archinstall kernels list).
func Kernels() []string {
	return []string{"linux", "linux-lts"}
}

// BaseSystemPackages are always pacstrapped in addition to profile packages.
func BaseSystemPackages() []string {
	return []string{
		"base",
		"linux-firmware",
		"btrfs-progs",
		"cryptsetup",
		"sudo",
		"efibootmgr",
		"dosfstools",
		"util-linux",
	}
}

// SharedExtraPackages is the exact package list from the former hz-install
// archinstall JSON (before the rgam5terra delta).
func SharedExtraPackages() []string {
	return []string{
		"base-devel",
		"git",
		"git-lfs",
		"openssh",
		"networkmanager",
		"networkmanager-openvpn",
		"nm-connection-editor",
		"openfortivpn",
		"greetd",
		"greetd-tuigreet",
		"sway",
		"swaybg",
		"swayidle",
		"swaylock",
		"waybar",
		"foot",
		"fuzzel",
		"grim",
		"slurp",
		"satty",
		"wf-recorder",
		"wl-clipboard",
		"cliphist",
		"brightnessctl",
		"pamixer",
		"playerctl",
		"polkit",
		"xdg-desktop-portal",
		"xdg-desktop-portal-wlr",
		"pipewire",
		"pipewire-audio",
		"pipewire-alsa",
		"pipewire-jack",
		"wireplumber",
		"pipewire-pulse",
		"gst-plugin-pipewire",
		"wob",
		"gammastep",
		"kanshi",
		"wdisplays",
		"wlr-randr",
		"firefox",
		"chromium",
		"vivaldi",
		"vivaldi-ffmpeg-codecs",
		"obsidian",
		"libreoffice-fresh",
		"zathura",
		"keepassxc",
		"gnome-keyring",
		"seahorse",
		"obs-studio",
		"kooha",
		"v4l2loopback-dkms",
		"v4l2loopback-utils",
		"vlc",
		"spotify-launcher",
		"element-desktop",
		"discord",
		"telegram-desktop",
		"thunar",
		"thunar-archive-plugin",
		"thunar-media-tags-plugin",
		"thunar-volman",
		"tumbler",
		"engrampa",
		"ffmpegthumbnailer",
		"gvfs-mtp",
		"android-file-transfer",
		"android-tools",
		"flatpak",
		"bluez-utils",
		"alsa-utils",
		"pavucontrol",
		"bash-completion",
		"ripgrep",
		"fd",
		"btop",
		"ncdu",
		"tree",
		"trash-cli",
		"unzip",
		"unrar",
		"yazi",
		"zellij",
		"github-cli",
		"glab",
		"shellcheck",
		"shfmt",
		"cmake",
		"meson",
		"gdb",
		"lldb",
		"strace",
		"python-pipx",
		"podman",
		"podman-compose",
		"reflector",
		"pacman-contrib",
		"noto-fonts",
		"noto-fonts-emoji",
		"ttf-jetbrains-mono-nerd",
		"otf-font-awesome",
		"xdg-utils",
		"chezmoi",
		"age",
		"pass",
		"fwupd",
		"smartmontools",
		"nvme-cli",
		"tailscale",
		"tlp",
		"tlp-rdw",
		"thermald",
		"zram-generator",
		"jq",
		"dkms",
		"linux-headers",
		"linux-lts-headers",
	}
}

// TerraRemovePackages are dropped on rgam5terra (laptop-only power stack).
func TerraRemovePackages() []string {
	return []string{"tlp", "tlp-rdw", "thermald"}
}

// TerraAddPackages are added on rgam5terra (desktop/GPU stack).
func TerraAddPackages() []string {
	return []string{
		"amd-ucode",
		"nvidia-open-dkms",
		"nvidia-utils",
		"nvidia-settings",
		"opencl-nvidia",
		"tuned",
		"tuned-ppd",
	}
}

// ExtraPackagesForProfile applies the rgam5terra package delta to the shared list.
func ExtraPackagesForProfile(profile string) []string {
	pkgs := append([]string(nil), SharedExtraPackages()...)
	if profile != ProfileAM5Terra {
		return pkgs
	}
	remove := map[string]struct{}{}
	for _, p := range TerraRemovePackages() {
		remove[p] = struct{}{}
	}
	out := make([]string, 0, len(pkgs)+len(TerraAddPackages()))
	for _, p := range pkgs {
		if _, drop := remove[p]; drop {
			continue
		}
		out = append(out, p)
	}
	out = append(out, TerraAddPackages()...)
	return out
}

// PacstrapPackages is the full set passed to pacstrap -K.
func PacstrapPackages(profile string) []string {
	out := append([]string(nil), BaseSystemPackages()...)
	out = append(out, Kernels()...)
	out = append(out, ExtraPackagesForProfile(profile)...)
	return out
}

// ContainsPackage reports whether name is in the profile extra package set
// (not base/kernel-only). Used by tests and selftests.
func ContainsPackage(profile, name string) bool {
	for _, p := range ExtraPackagesForProfile(profile) {
		if p == name {
			return true
		}
	}
	return false
}
