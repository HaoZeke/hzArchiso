package main

import (
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// (view helpers below use tea.View.Content)

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

func TestParseArgsAppliesRGSURFLatProfileDefaults(t *testing.T) {
	cfg, err := parseArgs([]string{
		"--profile", "rgSURFLat",
		"--target-disk", "/dev/nvme0n1",
	}, io.Discard)
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if cfg.profile != "rgSURFLat" {
		t.Fatalf("profile = %q, want rgSURFLat", cfg.profile)
	}
	if cfg.hostname != "rgSURFLat" {
		t.Fatalf("hostname = %q, want rgSURFLat", cfg.hostname)
	}
	if cfg.machineName != "rgSURFLat" {
		t.Fatalf("machineName = %q, want rgSURFLat", cfg.machineName)
	}
}

func TestParseArgsResolvesProfileIndex(t *testing.T) {
	// Catalog order: 1=rgx1gen11, 2=rgSURFLat, 3=rgam5terra
	cfg, err := parseArgs([]string{
		"--profile", "2",
		"--target-disk", "/dev/nvme0n1",
	}, io.Discard)
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if cfg.profile != profileRGSURFLat {
		t.Fatalf("profile = %q, want %s", cfg.profile, profileRGSURFLat)
	}
	if cfg.hostname != profileRGSURFLat {
		t.Fatalf("hostname = %q", cfg.hostname)
	}
}

func TestResolveProfileInputRejectsUnknown(t *testing.T) {
	if _, err := resolveProfileInput("nope"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := resolveProfileInput("99"); err == nil {
		t.Fatal("expected out-of-range error")
	}
}

func TestValidateFieldBlocksBadDiskAndProfile(t *testing.T) {
	if err := validateField(fieldDisk, "sda"); err == nil {
		t.Fatal("expected disk path error")
	}
	if err := validateField(fieldDisk, "/dev/nvme0n1"); err != nil {
		t.Fatal(err)
	}
	if err := validateField(fieldProfile, "nope"); err == nil {
		t.Fatal("expected profile error")
	}
	if err := validateField(fieldProfile, "1"); err != nil {
		t.Fatal(err)
	}
}

func TestInputStepRejectsInvalidFieldBeforeAdvance(t *testing.T) {
	m := newModel(testConfig())
	m.step = stepInput
	m.field = fieldProfile
	m.loadField()
	m.input.SetValue("not-a-profile")
	updated, _ := m.Update(keyPress("enter"))
	got := updated.(model)
	if got.field != fieldProfile {
		t.Fatalf("should stay on profile field, got %v", got.field)
	}
	if got.err == "" {
		t.Fatal("expected validation error on bad profile")
	}
	if got.step != stepInput {
		t.Fatalf("step = %v, want stepInput", got.step)
	}
}

func TestProfileIndexInTUIUpdatesDefaults(t *testing.T) {
	m := newModel(testConfig())
	m.field = fieldProfile
	m.loadField()
	m.input.SetValue("2")
	m.saveField()
	if m.cfg.profile != profileRGSURFLat {
		t.Fatalf("profile = %q", m.cfg.profile)
	}
	if m.cfg.hostname != profileRGSURFLat || m.cfg.machineName != profileRGSURFLat {
		t.Fatalf("hostname/machine = %q / %q", m.cfg.hostname, m.cfg.machineName)
	}
}

func TestWelcomeAndInputViewsExposeProfilesAndKeys(t *testing.T) {
	m := newModel(testConfig())
	welcome := viewContent(m)
	for _, need := range []string{"rgx1gen11", "rgSURFLat", "rgam5terra", "keys:", "Enter"} {
		if !strings.Contains(welcome, need) {
			t.Fatalf("welcome view missing %q in:\n%s", need, welcome)
		}
	}
	m.step = stepInput
	m.field = fieldProfile
	m.loadField()
	inputView := viewContent(m)
	for _, need := range []string{"Profiles", "1)", "Tab", "Shift-Tab"} {
		if !strings.Contains(inputView, need) {
			t.Fatalf("input view missing %q in:\n%s", need, inputView)
		}
	}
	m.step = stepReview
	review := viewContent(m)
	if !strings.Contains(review, "dry-run") || !strings.Contains(review, "exact disk") {
		t.Fatalf("review missing cues:\n%s", review)
	}
}

// viewContent extracts the text payload from the shipped View() path.
func viewContent(m model) string {
	return m.View().Content
}
