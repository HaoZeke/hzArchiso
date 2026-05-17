package main

import (
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
