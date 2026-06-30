package install

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	ProfileRGX1     = "rgx1gen11"
	ProfileAM5Terra = "rgam5terra"
	ProfileRGSURFLat = "rgSURFLat"

	DefaultUsername         = "rgoswami"
	DefaultTimezone         = "America/Chicago"
	DefaultConsoleKeymap    = "us"
	DefaultChezmoiKeyLayout = "colemak"
	DefaultOutputDir        = "/run/hz-install"

	// ESP is 1 GiB starting at 1 MiB (1 MiB alignment); root starts at 1025 MiB.
	ESPSizeMiB      = 1024
	ESPStartMiB     = 1
	RootStartMiB    = 1025
	GPTBackupTailMiB = 2

	MapperName = "cryptroot"
	Locale     = "en_US.UTF-8"
)

var (
	hostnameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,62}$`)
	usernameRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]*[$]?$`)
)

// Config is the install configuration collected by the TUI or CLI.
type Config struct {
	TargetDisk       string
	Profile          string
	Hostname         string
	Username         string
	Timezone         string
	ConsoleKeymap    string
	ChezmoiKeyLayout string
	MachineName      string
	OutputDir        string
	DryRun           bool
	// DiskBytes is optional; when 0, geometry uses an unknown root size (dry-run ok).
	DiskBytes int64
}

func DefaultConfig() Config {
	return Config{
		Profile:          ProfileRGX1,
		Hostname:         ProfileRGX1,
		Username:         DefaultUsername,
		Timezone:         DefaultTimezone,
		ConsoleKeymap:    DefaultConsoleKeymap,
		ChezmoiKeyLayout: DefaultChezmoiKeyLayout,
		MachineName:      ProfileRGX1,
		OutputDir:        DefaultOutputDir,
	}
}

func ValidProfile(profile string) bool {
	switch profile {
	case ProfileRGX1, ProfileAM5Terra, ProfileRGSURFLat:
		return true
	default:
		return false
	}
}

func ProfileDefaultName(profile string) (string, bool) {
	switch profile {
	case ProfileRGX1:
		return ProfileRGX1, true
	case ProfileAM5Terra:
		return ProfileAM5Terra, true
	case ProfileRGSURFLat:
		return ProfileRGSURFLat, true
	default:
		return "", false
	}
}

// ApplyProfileDefaults sets hostname/machine_name from profile when flags were omitted.
func ApplyProfileDefaults(cfg *Config, setHostname, setMachineName bool) error {
	defaultName, ok := ProfileDefaultName(cfg.Profile)
	if !ok {
		return fmt.Errorf("unsupported profile: %s", cfg.Profile)
	}
	if setHostname {
		cfg.Hostname = defaultName
	}
	if setMachineName {
		cfg.MachineName = defaultName
	}
	return nil
}

func ValidateConfig(cfg Config) error {
	switch {
	case !ValidProfile(cfg.Profile):
		return fmt.Errorf("unsupported profile: %s", cfg.Profile)
	case cfg.TargetDisk == "":
		return errors.New("--target-disk is required")
	case !strings.HasPrefix(cfg.TargetDisk, "/dev/"):
		return errors.New("--target-disk must be an absolute /dev path")
	case !hostnameRe.MatchString(cfg.Hostname):
		return fmt.Errorf("invalid hostname: %s", cfg.Hostname)
	case !usernameRe.MatchString(cfg.Username):
		return fmt.Errorf("invalid username: %s", cfg.Username)
	case strings.TrimSpace(cfg.Timezone) == "":
		return errors.New("--timezone is required")
	case strings.TrimSpace(cfg.ConsoleKeymap) == "":
		return errors.New("--console-keymap is required")
	case strings.TrimSpace(cfg.ChezmoiKeyLayout) == "":
		return errors.New("--chezmoi-key-layout is required")
	case strings.TrimSpace(cfg.MachineName) == "":
		return errors.New("--machine-name is required")
	case strings.TrimSpace(cfg.OutputDir) == "":
		return errors.New("--output-dir is required")
	default:
		return nil
	}
}

// RootSizeMiB computes the LUKS root partition size in MiB from disk bytes.
// Returns 0 when disk size is unknown or too small.
func RootSizeMiB(diskBytes int64) int64 {
	if diskBytes <= 0 {
		return 0
	}
	totalMiB := diskBytes / (1024 * 1024)
	root := totalMiB - RootStartMiB - GPTBackupTailMiB
	if root <= 0 {
		return 0
	}
	return root
}

// PartitionPaths returns ESP and root partition device paths for whole disks.
func PartitionPaths(disk string) (esp, root string) {
	if strings.HasPrefix(disk, "/dev/nvme") || strings.HasPrefix(disk, "/dev/mmcblk") || strings.HasPrefix(disk, "/dev/loop") {
		return disk + "p1", disk + "p2"
	}
	return disk + "1", disk + "2"
}

func MapperPath() string {
	return "/dev/mapper/" + MapperName
}

// HaveCUDA is the chezmoi have_cuda flag for the profile.
func HaveCUDA(profile string) string {
	if profile == ProfileAM5Terra {
		return "yes"
	}
	return "no"
}
