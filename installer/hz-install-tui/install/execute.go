package install

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/term"
)

const mnt = "/mnt"

// Runner executes external commands; tests may substitute a recorder.
type Runner interface {
	Run(name string, args ...string) error
	RunInput(stdin string, name string, args ...string) error
}

// ExecRunner runs real OS commands.
type ExecRunner struct {
	Stdout io.Writer
	Stderr io.Writer
}

func (r ExecRunner) Run(name string, args ...string) error {
	return r.RunInput("", name, args...)
}

func (r ExecRunner) RunInput(stdin string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	} else {
		cmd.Stdin = os.Stdin
	}
	if r.Stdout != nil {
		cmd.Stdout = r.Stdout
	} else {
		cmd.Stdout = os.Stdout
	}
	if r.Stderr != nil {
		cmd.Stderr = r.Stderr
	} else {
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

// Secrets holds interactive passwords for a real install.
type Secrets struct {
	RootPassword string
	UserPassword string
	DiskPassword string
}

// ReadSecrets prompts on the terminal for install passwords.
func ReadSecrets(username string, in *os.File, out io.Writer) (Secrets, error) {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stderr
	}
	read := func(prompt string) (string, error) {
		fmt.Fprint(out, prompt)
		fd := int(in.Fd())
		if term.IsTerminal(fd) {
			b, err := term.ReadPassword(fd)
			fmt.Fprintln(out)
			if err != nil {
				return "", err
			}
			return string(b), nil
		}
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && len(strings.TrimSpace(line)) == 0 {
			return "", err
		}
		return strings.TrimSpace(line), nil
	}
	var s Secrets
	var err error
	if s.RootPassword, err = read("Root password: "); err != nil {
		return s, err
	}
	if s.UserPassword, err = read(fmt.Sprintf("User password for %s: ", username)); err != nil {
		return s, err
	}
	if s.DiskPassword, err = read("Disk encryption password: "); err != nil {
		return s, err
	}
	if s.RootPassword == "" || s.UserPassword == "" || s.DiskPassword == "" {
		return s, fmt.Errorf("passwords must be non-empty")
	}
	return s, nil
}

// ConfirmDisk requires the operator to type the exact disk path.
func ConfirmDisk(disk string, in io.Reader, out io.Writer) error {
	if out == nil {
		out = os.Stderr
	}
	if in == nil {
		in = os.Stdin
	}
	fmt.Fprintf(out, "This will erase %s. Type the exact disk path to continue: ", disk)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && len(strings.TrimSpace(line)) == 0 {
		return fmt.Errorf("confirmation failed")
	}
	if strings.TrimSpace(line) != disk {
		return fmt.Errorf("confirmation did not match target disk")
	}
	return nil
}

// DiskSizeBytes returns the size of a block device in bytes.
func DiskSizeBytes(disk string) (int64, error) {
	out, err := exec.Command("blockdev", "--getsize64", disk).Output()
	if err == nil {
		n, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		if err == nil && n > 0 {
			return n, nil
		}
	}
	out, err = exec.Command("lsblk", "-bdno", "SIZE", disk).Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
}

// Run executes dry-run (print plan) or the full install sequence.
func Run(cfg Config, runner Runner, secrets Secrets, stdin io.Reader, stdout, stderr io.Writer) error {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	if runner == nil {
		runner = ExecRunner{Stdout: stdout, Stderr: stderr}
	}

	if !cfg.DryRun {
		if os.Geteuid() != 0 {
			return fmt.Errorf("non-dry-run install must run as root")
		}
		var sstat syscall.Stat_t
		if err := syscall.Stat(cfg.TargetDisk, &sstat); err != nil || (sstat.Mode&syscall.S_IFMT) != syscall.S_IFBLK {
			return fmt.Errorf("target disk is not a block device: %s", cfg.TargetDisk)
		}
		if cfg.DiskBytes == 0 {
			n, err := DiskSizeBytes(cfg.TargetDisk)
			if err != nil || n <= 0 {
				return fmt.Errorf("could not determine size of %s", cfg.TargetDisk)
			}
			cfg.DiskBytes = n
		}
		if RootSizeMiB(cfg.DiskBytes) <= 0 {
			return fmt.Errorf("could not determine size of %s", cfg.TargetDisk)
		}
	}

	plan, err := BuildPlan(cfg)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(cfg.OutputDir, 0o700); err != nil {
		return err
	}
	planPath := filepath.Join(cfg.OutputDir, "install-plan.txt")
	f, err := os.OpenFile(planPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := FormatPlan(io.MultiWriter(stdout, f), plan); err != nil {
		_ = f.Close()
		return err
	}
	_ = f.Close()
	fmt.Fprintf(stdout, "wrote %s\n", planPath)

	if cfg.DryRun {
		return nil
	}

	if err := ConfirmDisk(cfg.TargetDisk, stdin, stderr); err != nil {
		return err
	}
	if secrets.RootPassword == "" {
		var err error
		secrets, err = ReadSecrets(cfg.Username, os.Stdin, stderr)
		if err != nil {
			return err
		}
	}

	return executeInstall(plan, secrets, runner, stderr)
}

func executeInstall(p Plan, secrets Secrets, runner Runner, stderr io.Writer) error {
	cfg := p.Config
	disk := cfg.TargetDisk
	esp, rootPart := p.ESPDev, p.RootDev
	mapper := MapperName
	mapperPath := p.Mapper

	if err := runner.Run("sgdisk", "--zap-all", disk); err != nil {
		return err
	}
	if err := runner.Run("sgdisk",
		"-n", fmt.Sprintf("1:%dM:+%dM", ESPStartMiB, ESPSizeMiB),
		"-t", "1:ef00",
		"-c", "1:ESP",
		disk,
	); err != nil {
		return err
	}
	if p.RootSizeMiB > 0 {
		if err := runner.Run("sgdisk",
			"-n", fmt.Sprintf("2:%dM:+%dM", RootStartMiB, p.RootSizeMiB),
			"-t", "2:8309",
			"-c", "2:cryptroot",
			disk,
		); err != nil {
			return err
		}
	} else {
		if err := runner.Run("sgdisk",
			"-n", fmt.Sprintf("2:%dM:0", RootStartMiB),
			"-t", "2:8309",
			"-c", "2:cryptroot",
			disk,
		); err != nil {
			return err
		}
	}
	_ = runner.Run("partprobe", disk)
	_ = runner.Run("udevadm", "settle")

	if err := runner.RunInput(secrets.DiskPassword+"\n", "cryptsetup", "-q", "luksFormat", rootPart); err != nil {
		return fmt.Errorf("luksFormat: %w", err)
	}
	if err := runner.RunInput(secrets.DiskPassword+"\n", "cryptsetup", "open", rootPart, mapper); err != nil {
		return fmt.Errorf("luks open: %w", err)
	}

	if err := runner.Run("mkfs.fat", "-F32", esp); err != nil {
		return err
	}
	if err := runner.Run("mkfs.btrfs", "-f", mapperPath); err != nil {
		return err
	}

	if err := runner.Run("mount", mapperPath, mnt); err != nil {
		return err
	}
	for _, s := range p.Subvolumes {
		if err := runner.Run("btrfs", "subvolume", "create", filepath.Join(mnt, s.Name)); err != nil {
			return err
		}
	}
	if err := runner.Run("umount", mnt); err != nil {
		return err
	}

	if err := runner.Run("mount", "-o", p.MountOptions+",subvol=@", mapperPath, mnt); err != nil {
		return err
	}
	for _, s := range p.Subvolumes {
		if s.Name == "@" {
			continue
		}
		target := filepath.Join(mnt, strings.TrimPrefix(s.Mountpoint, "/"))
		if err := os.MkdirAll(target, 0o755); err != nil {
			return err
		}
		if err := runner.Run("mount", "-o", p.MountOptions+",subvol="+s.Name, mapperPath, target); err != nil {
			return err
		}
	}
	boot := filepath.Join(mnt, "boot")
	if err := os.MkdirAll(boot, 0o755); err != nil {
		return err
	}
	if err := runner.Run("mount", esp, boot); err != nil {
		return err
	}

	args := append([]string{"-K", mnt}, p.Packages...)
	if err := runner.Run("pacstrap", args...); err != nil {
		return err
	}

	fstabCmd := exec.Command("genfstab", "-U", mnt)
	fstabOut, err := fstabCmd.Output()
	if err != nil {
		return fmt.Errorf("genfstab: %w", err)
	}
	fstabPath := filepath.Join(mnt, "etc", "fstab")
	if err := os.WriteFile(fstabPath, fstabOut, 0o644); err != nil {
		return err
	}

	script := buildChrootScript(p, secrets)
	scriptPath := filepath.Join(mnt, "root", "hz-chroot-setup.sh")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		return err
	}
	if err := runner.Run("arch-chroot", mnt, "/root/hz-chroot-setup.sh"); err != nil {
		return err
	}
	_ = os.Remove(scriptPath)

	fmt.Fprintln(stderr, "install complete; reboot when ready")
	return nil
}

func buildChrootScript(p Plan, secrets Secrets) string {
	cfg := p.Config
	var b strings.Builder
	b.WriteString("#!/bin/bash\nset -euo pipefail\n")
	fmt.Fprintf(&b, "ln -sf /usr/share/zoneinfo/%s /etc/localtime\n", cfg.Timezone)
	b.WriteString("hwclock --systohc\n")
	fmt.Fprintf(&b, "grep -q '^%s' /etc/locale.gen || echo '%s UTF-8' >> /etc/locale.gen\n", Locale, Locale)
	fmt.Fprintf(&b, "sed -i 's/^#%s/%s/' /etc/locale.gen || true\n", Locale, Locale)
	b.WriteString("locale-gen\n")
	fmt.Fprintf(&b, "echo LANG=%s > /etc/locale.conf\n", Locale)
	fmt.Fprintf(&b, "echo KEYMAP=%s > /etc/vconsole.conf\n", cfg.ConsoleKeymap)
	fmt.Fprintf(&b, "echo %s > /etc/hostname\n", cfg.Hostname)
	fmt.Fprintf(&b, "printf '127.0.0.1 localhost\\n::1 localhost\\n127.0.1.1 %s.localdomain %s\\n' > /etc/hosts\n", cfg.Hostname, cfg.Hostname)
	b.WriteString("chpasswd <<'PASSWD_ROOT'\n")
	fmt.Fprintf(&b, "root:%s\n", secrets.RootPassword)
	b.WriteString("PASSWD_ROOT\n")
	fmt.Fprintf(&b, "id -u %s >/dev/null 2>&1 || useradd -m -G wheel -s /bin/bash %s\n", cfg.Username, cfg.Username)
	b.WriteString("chpasswd <<'PASSWD_USER'\n")
	fmt.Fprintf(&b, "%s:%s\n", cfg.Username, secrets.UserPassword)
	b.WriteString("PASSWD_USER\n")
	b.WriteString("echo '%wheel ALL=(ALL:ALL) ALL' > /etc/sudoers.d/wheel\n")
	b.WriteString("chmod 440 /etc/sudoers.d/wheel\n")
	b.WriteString("HOOKS_LINE='HOOKS=(base udev autodetect microcode modconf kms keyboard keymap consolefont block encrypt btrfs filesystems fsck)'\n")
	b.WriteString("if grep -q '^HOOKS=' /etc/mkinitcpio.conf; then sed -i \"s/^HOOKS=.*/${HOOKS_LINE}/\" /etc/mkinitcpio.conf; else echo \"$HOOKS_LINE\" >> /etc/mkinitcpio.conf; fi\n")
	b.WriteString("mkinitcpio -P\n")
	fmt.Fprintf(&b, "ROOT_UUID=$(blkid -s UUID -o value %s)\n", p.RootDev)
	b.WriteString("bootctl install\n")
	b.WriteString("install -d /boot/loader/entries\n")
	b.WriteString("cat > /boot/loader/loader.conf <<'LCONF'\n")
	b.WriteString("default arch.conf\ntimeout 3\nconsole-mode max\neditor no\nLCONF\n")
	fmt.Fprintf(&b, "cat > /boot/loader/entries/arch.conf <<EOF\ntitle Arch Linux\nlinux /vmlinuz-linux\ninitrd /initramfs-linux.img\noptions cryptdevice=UUID=${ROOT_UUID}:%s root=%s rootflags=subvol=@ rw\nEOF\n", MapperName, p.Mapper)
	fmt.Fprintf(&b, "cat > /boot/loader/entries/arch-lts.conf <<EOF\ntitle Arch Linux LTS\nlinux /vmlinuz-linux-lts\ninitrd /initramfs-linux-lts.img\noptions cryptdevice=UUID=${ROOT_UUID}:%s root=%s rootflags=subvol=@ rw\nEOF\n", MapperName, p.Mapper)
	b.WriteString("systemctl enable NetworkManager.service\n")
	fmt.Fprintf(&b, "install -d -m 0755 /home/%s/.config/chezmoi\n", cfg.Username)
		fmt.Fprintf(&b, "cat > /home/%s/.config/chezmoi/chezmoi.toml <<'CTOML'\n", cfg.Username)
	b.WriteString("[data]\n")
	b.WriteString("display_manager = \"sway\"\n")
	fmt.Fprintf(&b, "key_layout = \"%s\"\n", cfg.ChezmoiKeyLayout)
	b.WriteString("encrypted = \"yes\"\n")
	fmt.Fprintf(&b, "machine_name = \"%s\"\n", cfg.MachineName)
	b.WriteString("setup_mamba = \"no\"\n")
	b.WriteString("setup_pixi = \"yes\"\n")
	b.WriteString("setup_spack = \"no\"\n")
	b.WriteString("setup_nix = \"no\"\n")
	b.WriteString("use_rust = \"yes\"\n")
	fmt.Fprintf(&b, "have_cuda = \"%s\"\n", HaveCUDA(cfg.Profile))
	b.WriteString("CTOML\n")
	fmt.Fprintf(&b, "chown -R %s:%s /home/%s/.config\n", cfg.Username, cfg.Username, cfg.Username)
	fmt.Fprintf(&b, "sudo -u %s chezmoi init --apply HaoZeke --branch chezmoi\n", cfg.Username)
	return b.String()
}
