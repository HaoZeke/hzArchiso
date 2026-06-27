package install

import (
	"fmt"
	"io"
	"strings"
)

// Step is one planned install action (dry-run prints these; execute runs them).
type Step struct {
	Name    string
	Detail  string
	Command []string // optional illustrative argv; may be empty for multi-step ops
}

// Plan is the full install plan derived from Config.
type Plan struct {
	Config       Config
	ESPDev       string
	RootDev      string
	Mapper       string
	RootSizeMiB  int64
	Packages     []string
	ExtraPkgs    []string
	Subvolumes   []Subvolume
	MountOptions string
	Steps        []Step
	CustomCmds   []string
	KernelParams string
}

// Subvolume is a btrfs subvolume mount layout entry.
type Subvolume struct {
	Name       string
	Mountpoint string
}

func DefaultSubvolumes() []Subvolume {
	return []Subvolume{
		{Name: "@", Mountpoint: "/"},
		{Name: "@home", Mountpoint: "/home"},
		{Name: "@snapshots", Mountpoint: "/.snapshots"},
		{Name: "@var_log", Mountpoint: "/var/log"},
		{Name: "@cache", Mountpoint: "/var/cache/pacman/pkg"},
	}
}

// BuildPlan constructs the install plan. Does not touch disks.
func BuildPlan(cfg Config) (Plan, error) {
	if err := ValidateConfig(cfg); err != nil {
		return Plan{}, err
	}
	esp, root := PartitionPaths(cfg.TargetDisk)
	rootMiB := RootSizeMiB(cfg.DiskBytes)
	subs := DefaultSubvolumes()
	extra := ExtraPackagesForProfile(cfg.Profile)
	pkgs := PacstrapPackages(cfg.Profile)
	mapper := MapperPath()
	mountOpts := "compress=zstd,noatime"
	kernelParams := fmt.Sprintf(
		"cryptdevice=UUID=ROOT_PART_UUID:%s root=%s rootflags=subvol=@ rw",
		MapperName, mapper,
	)

	custom := CustomCommands(cfg)
	steps := []Step{
		{Name: "partition", Detail: fmt.Sprintf("sgdisk GPT on %s: ESP %d MiB type ef00 @ %d MiB, root type 8309 from %d MiB (size %d MiB or fill)",
			cfg.TargetDisk, ESPSizeMiB, ESPStartMiB, RootStartMiB, rootMiB),
			Command: []string{"sgdisk", "--zap-all", cfg.TargetDisk}},
		{Name: "luks-format", Detail: fmt.Sprintf("cryptsetup luksFormat %s", root)},
		{Name: "luks-open", Detail: fmt.Sprintf("cryptsetup open %s %s", root, MapperName)},
		{Name: "mkfs-esp", Detail: fmt.Sprintf("mkfs.fat -F32 %s", esp)},
		{Name: "mkfs-btrfs", Detail: fmt.Sprintf("mkfs.btrfs -f %s", mapper)},
		{Name: "subvolumes", Detail: "create btrfs subvolumes @ @home @snapshots @var_log @cache"},
		{Name: "mount", Detail: fmt.Sprintf("mount %s with %s; ESP at /boot", mountOpts, "subvol=@")},
		{Name: "pacstrap", Detail: fmt.Sprintf("pacstrap -K /mnt (%d packages incl. base, kernels, profile)", len(pkgs))},
		{Name: "genfstab", Detail: "genfstab -U /mnt >> /mnt/etc/fstab"},
		{Name: "timezone", Detail: "arch-chroot: timedatectl / ln -sf /usr/share/zoneinfo/" + cfg.Timezone},
		{Name: "locale", Detail: "locale " + Locale + "; KEYMAP=" + cfg.ConsoleKeymap},
		{Name: "hostname", Detail: cfg.Hostname},
		{Name: "users", Detail: fmt.Sprintf("root password; user %s with sudo", cfg.Username)},
		{Name: "mkinitcpio", Detail: "HOOKS include encrypt btrfs (or sd-encrypt for systemd)"},
		{Name: "bootloader", Detail: "bootctl install; systemd-boot entries with " + kernelParams},
		{Name: "networkmanager", Detail: "systemctl enable NetworkManager.service"},
		{Name: "chezmoi", Detail: "chezmoi.toml + chezmoi init --apply HaoZeke --branch chezmoi"},
	}

	return Plan{
		Config:       cfg,
		ESPDev:       esp,
		RootDev:      root,
		Mapper:       mapper,
		RootSizeMiB:  rootMiB,
		Packages:     pkgs,
		ExtraPkgs:    extra,
		Subvolumes:   subs,
		MountOptions: mountOpts,
		Steps:        steps,
		CustomCmds:   custom,
		KernelParams: kernelParams,
	}, nil
}

// CustomCommands ports the archinstall custom-commands block exactly in spirit.
func CustomCommands(cfg Config) []string {
	haveCUDA := HaveCUDA(cfg.Profile)
	chezmoiTOML := strings.Join([]string{
		"[data]",
		`display_manager = "sway"`,
		fmt.Sprintf(`key_layout = "%s"`, cfg.ChezmoiKeyLayout),
		`encrypted = "yes"`,
		fmt.Sprintf(`machine_name = "%s"`, cfg.MachineName),
		`setup_mamba = "no"`,
		`setup_pixi = "yes"`,
		`setup_spack = "no"`,
		`setup_nix = "no"`,
		`use_rust = "yes"`,
		fmt.Sprintf(`have_cuda = "%s"`, haveCUDA),
		"",
	}, "\n")
	return []string{
		"systemctl enable NetworkManager.service",
		fmt.Sprintf("install -d -m 0755 /home/%s/.config/chezmoi", cfg.Username),
		fmt.Sprintf("write /home/%s/.config/chezmoi/chezmoi.toml:\n%s", cfg.Username, chezmoiTOML),
		fmt.Sprintf("chown -R %s:%s /home/%s/.config", cfg.Username, cfg.Username, cfg.Username),
		fmt.Sprintf("sudo -u %s chezmoi init --apply HaoZeke --branch chezmoi", cfg.Username),
	}
}

// FormatPlan writes a human-readable plan suitable for --dry-run and selftests.
func FormatPlan(w io.Writer, p Plan) error {
	cfg := p.Config
	printf := func(format string, args ...any) error {
		_, err := fmt.Fprintf(w, format, args...)
		return err
	}
	if err := printf("hz-install plan (native Go, no archinstall)\n"); err != nil {
		return err
	}
	if err := printf("profile=%s hostname=%s username=%s timezone=%s\n", cfg.Profile, cfg.Hostname, cfg.Username, cfg.Timezone); err != nil {
		return err
	}
	if err := printf("console_keymap=%s chezmoi_key_layout=%s machine_name=%s have_cuda=%s\n",
		cfg.ConsoleKeymap, cfg.ChezmoiKeyLayout, cfg.MachineName, HaveCUDA(cfg.Profile)); err != nil {
		return err
	}
	if err := printf("target_disk=%s esp=%s root_part=%s mapper=%s\n", cfg.TargetDisk, p.ESPDev, p.RootDev, p.Mapper); err != nil {
		return err
	}
	if err := printf("gpt: ESP %d MiB type ef00 starting %d MiB; root LUKS type 8309 starting %d MiB",
		ESPSizeMiB, ESPStartMiB, RootStartMiB); err != nil {
		return err
	}
	if p.RootSizeMiB > 0 {
		if err := printf(" size=%d MiB\n", p.RootSizeMiB); err != nil {
			return err
		}
	} else {
		if err := printf(" size=fill-remainder (disk size unknown or dry-run without blockdev)\n"); err != nil {
			return err
		}
	}
	if err := printf("filesystems: mkfs.fat -F32 ESP; cryptsetup luksFormat+open root; mkfs.btrfs mapper\n"); err != nil {
		return err
	}
	if err := printf("btrfs subvolumes:"); err != nil {
		return err
	}
	for _, s := range p.Subvolumes {
		if err := printf(" %s->%s", s.Name, s.Mountpoint); err != nil {
			return err
		}
	}
	if err := printf("\n"); err != nil {
		return err
	}
	if err := printf("mount_options=%s esp_mount=/boot\n", p.MountOptions); err != nil {
		return err
	}
	if err := printf("locale=%s\n", Locale); err != nil {
		return err
	}
	if err := printf("kernels: %s\n", strings.Join(Kernels(), " ")); err != nil {
		return err
	}
	if err := printf("bootloader=systemd-boot kernel_params=%s\n", p.KernelParams); err != nil {
		return err
	}
	if err := printf("packages (%d total pacstrap, %d profile extras):\n", len(p.Packages), len(p.ExtraPkgs)); err != nil {
		return err
	}
	for _, pkg := range p.ExtraPkgs {
		if err := printf("  - %s\n", pkg); err != nil {
			return err
		}
	}
	if err := printf("steps:\n"); err != nil {
		return err
	}
	for i, st := range p.Steps {
		if err := printf("  %2d. %s: %s\n", i+1, st.Name, st.Detail); err != nil {
			return err
		}
	}
	if err := printf("custom-commands:\n"); err != nil {
		return err
	}
	for _, c := range p.CustomCmds {
		if err := printf("  * %s\n", c); err != nil {
			return err
		}
	}
	if cfg.DryRun {
		if err := printf("dry-run: no sgdisk/cryptsetup/mkfs/pacstrap executed\n"); err != nil {
			return err
		}
	}
	return nil
}
