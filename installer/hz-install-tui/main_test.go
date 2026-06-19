package main

import (
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func testConfig() installConfig {
	cfg := defaultConfig()
	cfg.targetDisk = "/dev/nvme0n1"
	return cfg
}

func keyPress(name string) tea.KeyPressMsg {
	switch name {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	default:
		return tea.KeyPressMsg{Text: name, Code: []rune(name)[0]}
	}
}

func TestInstallActionRequiresExactDiskConfirmation(t *testing.T) {
	m := newModel(testConfig())
	m.step = stepReview

	updated, _ := m.Update(keyPress("i"))
	got := updated.(model)

	if got.action != actionNone {
		t.Fatalf("install action should not run before confirmation, got %v", got.action)
	}
	if got.step == stepReview {
		t.Fatalf("install request should move to a confirmation step")
	}
}

func TestInstallConfirmationRejectsMismatchedDisk(t *testing.T) {
	m := newModel(testConfig())
	m.step = stepReview
	updated, _ := m.Update(keyPress("i"))
	confirming := updated.(model)
	confirming.input.SetValue("/dev/sda")

	updated, _ = confirming.Update(keyPress("enter"))
	got := updated.(model)

	if got.action != actionNone {
		t.Fatalf("mismatched confirmation should not run install, got %v", got.action)
	}
	if !strings.Contains(got.err, "confirmation did not match target disk") {
		t.Fatalf("expected mismatch error, got %q", got.err)
	}
}

func TestInstallConfirmationAcceptsExactDisk(t *testing.T) {
	m := newModel(testConfig())
	m.step = stepReview
	updated, _ := m.Update(keyPress("i"))
	confirming := updated.(model)
	confirming.input.SetValue("/dev/nvme0n1")

	updated, _ = confirming.Update(keyPress("enter"))
	got := updated.(model)

	if got.action != actionInstall {
		t.Fatalf("exact confirmation should run install, got %v", got.action)
	}
}

func TestParseArgsAppliesTerraProfileDefaults(t *testing.T) {
	cfg, err := parseArgs([]string{
		"--profile", "rgam5terra",
		"--target-disk", "/dev/nvme0n1",
	}, io.Discard)
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	if cfg.profile != "rgam5terra" {
		t.Fatalf("profile = %q, want rgam5terra", cfg.profile)
	}
	if cfg.hostname != "rgam5terra" {
		t.Fatalf("hostname = %q, want rgam5terra", cfg.hostname)
	}
	if cfg.machineName != "rgam5terra" {
		t.Fatalf("machineName = %q, want rgam5terra", cfg.machineName)
	}
}

func TestParseArgsKeepsExplicitProfileNames(t *testing.T) {
	cfg, err := parseArgs([]string{
		"--profile", "rgam5terra",
		"--target-disk", "/dev/nvme0n1",
		"--hostname", "workstation",
		"--machine-name", "rgx1gen11",
	}, io.Discard)
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	if cfg.hostname != "workstation" {
		t.Fatalf("hostname = %q, want workstation", cfg.hostname)
	}
	if cfg.machineName != "rgx1gen11" {
		t.Fatalf("machineName = %q, want rgx1gen11", cfg.machineName)
	}
}

func TestProfileFieldUpdatesHostDefaults(t *testing.T) {
	m := newModel(testConfig())
	m.field = fieldProfile
	m.loadField()
	m.input.SetValue("rgam5terra")
	m.saveField()

	if m.cfg.profile != "rgam5terra" {
		t.Fatalf("profile = %q, want rgam5terra", m.cfg.profile)
	}
	if m.cfg.hostname != "rgam5terra" {
		t.Fatalf("hostname = %q, want rgam5terra", m.cfg.hostname)
	}
	if m.cfg.machineName != "rgam5terra" {
		t.Fatalf("machineName = %q, want rgam5terra", m.cfg.machineName)
	}
}
