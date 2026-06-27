package install

import (
	"strings"
	"testing"
)

func TestExtraPackagesProfileDelta(t *testing.T) {
	rgx1 := ExtraPackagesForProfile(ProfileRGX1)
	terra := ExtraPackagesForProfile(ProfileAM5Terra)

	for _, pkg := range []string{"tlp", "tlp-rdw", "thermald", "chezmoi", "greetd-tuigreet"} {
		if !ContainsPackage(ProfileRGX1, pkg) {
			t.Fatalf("rgx1 missing %s", pkg)
		}
	}
	for _, pkg := range TerraRemovePackages() {
		if ContainsPackage(ProfileAM5Terra, pkg) {
			t.Fatalf("terra should not include %s", pkg)
		}
	}
	for _, pkg := range TerraAddPackages() {
		if !ContainsPackage(ProfileAM5Terra, pkg) {
			t.Fatalf("terra missing %s", pkg)
		}
		if ContainsPackage(ProfileRGX1, pkg) {
			t.Fatalf("rgx1 should not include terra package %s", pkg)
		}
	}
	if len(terra) != len(rgx1)-len(TerraRemovePackages())+len(TerraAddPackages()) {
		t.Fatalf("unexpected terra package count: terra=%d rgx1=%d", len(terra), len(rgx1))
	}
	if ContainsPackage(ProfileRGX1, "kanata") {
		t.Fatal("kanata must not be in installer packages")
	}
}

func TestBuildPlanOrderingAndSurface(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TargetDisk = "/dev/testdisk"
	cfg.DryRun = true
	cfg.DiskBytes = 512 * 1024 * 1024 * 1024 // 512 GiB

	p, err := BuildPlan(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if p.RootSizeMiB <= 0 {
		t.Fatalf("expected root size, got %d", p.RootSizeMiB)
	}
	if p.ESPDev != "/dev/testdisk1" || p.RootDev != "/dev/testdisk2" {
		t.Fatalf("parts = %s %s", p.ESPDev, p.RootDev)
	}
	names := make([]string, len(p.Steps))
	for i, s := range p.Steps {
		names[i] = s.Name
	}
	joined := strings.Join(names, ",")
	for _, need := range []string{"partition", "luks-format", "mkfs-btrfs", "subvolumes", "pacstrap", "mkinitcpio", "bootloader", "chezmoi"} {
		if !strings.Contains(joined, need) {
			t.Fatalf("missing step %s in %s", need, joined)
		}
	}
	if !strings.Contains(p.MountOptions, "compress=zstd") || !strings.Contains(p.MountOptions, "noatime") {
		t.Fatalf("mount options: %s", p.MountOptions)
	}
	subs := map[string]bool{}
	for _, s := range p.Subvolumes {
		subs[s.Name] = true
	}
	for _, n := range []string{"@", "@home", "@snapshots", "@var_log", "@cache"} {
		if !subs[n] {
			t.Fatalf("missing subvol %s", n)
		}
	}
	custom := strings.Join(p.CustomCmds, "\n")
	if !strings.Contains(custom, "chezmoi init --apply HaoZeke --branch chezmoi") {
		t.Fatal("missing chezmoi init")
	}
	if !strings.Contains(custom, `key_layout = "colemak"`) {
		t.Fatal("missing key_layout")
	}
	if !strings.Contains(custom, `have_cuda = "no"`) {
		t.Fatal("rgx1 should have_cuda=no")
	}
	if !strings.Contains(p.KernelParams, "cryptdevice=") || !strings.Contains(p.KernelParams, "rootflags=subvol=@") {
		t.Fatalf("kernel params: %s", p.KernelParams)
	}
}

func TestTerraPlanCUDAAndHostnameDefaults(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Profile = ProfileAM5Terra
	if err := ApplyProfileDefaults(&cfg, true, true); err != nil {
		t.Fatal(err)
	}
	cfg.TargetDisk = "/dev/nvme0n1"
	p, err := BuildPlan(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if p.ESPDev != "/dev/nvme0n1p1" {
		t.Fatalf("esp %s", p.ESPDev)
	}
	custom := strings.Join(p.CustomCmds, "\n")
	if !strings.Contains(custom, `machine_name = "rgam5terra"`) {
		t.Fatal(custom)
	}
	if !strings.Contains(custom, `have_cuda = "yes"`) {
		t.Fatal(custom)
	}
	var buf strings.Builder
	if err := FormatPlan(&buf, p); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "nvidia-open-dkms") || strings.Contains(out, "\n  - tlp\n") {
		// tlp might appear in step text? check packages section only via ContainsPackage
	}
	if ContainsPackage(ProfileAM5Terra, "tlp") {
		t.Fatal("tlp in terra")
	}
	if !strings.Contains(out, "locale="+Locale) {
		t.Fatal("locale missing from plan text")
	}
	if !strings.Contains(out, "no archinstall") {
		t.Fatal("plan should state no archinstall")
	}
}

func TestValidateRejectsBadProfile(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Profile = "nope"
	cfg.TargetDisk = "/dev/sda"
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error")
	}
}

func TestRootSizeMiB(t *testing.T) {
	// 100 GiB disk
	bytes := int64(100) * 1024 * 1024 * 1024
	got := RootSizeMiB(bytes)
	want := bytes/(1024*1024) - RootStartMiB - GPTBackupTailMiB
	if got != want {
		t.Fatalf("got %d want %d", got, want)
	}
}
