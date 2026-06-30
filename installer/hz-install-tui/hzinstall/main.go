package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/HaoZeke/hzArchiso/installer/hz-install-tui/install"
)

func main() {
	cfg := install.DefaultConfig()
	var (
		hostnameSet    bool
		machineNameSet bool
	)
	fs := flag.NewFlagSet("hz-install", flag.ExitOnError)
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "print the install plan but do not install")
	fs.StringVar(&cfg.Profile, "profile", cfg.Profile, "machine profile: rgx1gen11, rgam5terra, or rgSURFLat")
	fs.StringVar(&cfg.TargetDisk, "target-disk", cfg.TargetDisk, "whole disk to partition")
	fs.Func("hostname", "installed hostname", func(s string) error {
		cfg.Hostname = s
		hostnameSet = true
		return nil
	})
	fs.StringVar(&cfg.Username, "username", cfg.Username, "primary sudo user")
	fs.StringVar(&cfg.Timezone, "timezone", cfg.Timezone, "installed timezone")
	fs.StringVar(&cfg.ConsoleKeymap, "console-keymap", cfg.ConsoleKeymap, "console keymap")
	fs.StringVar(&cfg.ChezmoiKeyLayout, "chezmoi-key-layout", cfg.ChezmoiKeyLayout, "chezmoi key_layout value")
	fs.Func("machine-name", "chezmoi machine_name value", func(s string) error {
		cfg.MachineName = s
		machineNameSet = true
		return nil
	})
	fs.StringVar(&cfg.OutputDir, "output-dir", cfg.OutputDir, "directory for install plan output")
	// defaults for hostname/machine when flags not set — apply after parse
	_ = fs.Parse(os.Args[1:])

	// Re-apply defaults if profile flags omitted — use Visit to detect
	seenHost, seenMachine := false, false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "hostname":
			seenHost = true
		case "machine-name":
			seenMachine = true
		}
	})
	_ = hostnameSet
	_ = machineNameSet
	if err := install.ApplyProfileDefaults(&cfg, !seenHost, !seenMachine); err != nil {
		die(err)
	}
	if err := install.ValidateConfig(cfg); err != nil {
		die(err)
	}

	if err := install.Run(cfg, nil, install.Secrets{}, os.Stdin, os.Stdout, os.Stderr); err != nil {
		die(err)
	}
}

func die(err error) {
	fmt.Fprintf(os.Stderr, "hz-install: %v\n", err)
	if strings.Contains(err.Error(), "confirmation") || strings.Contains(err.Error(), "must run as root") {
		os.Exit(1)
	}
	os.Exit(1)
}
