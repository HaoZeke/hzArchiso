package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const defaultBackend = "/usr/local/bin/hz-install"

var (
	hostnameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,62}$`)
	usernameRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]*[$]?$`)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00A884"))
	warnStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#D97706"))
	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280"))
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#00A884")).
			Padding(1, 2).
			Width(76)
)

type installConfig struct {
	targetDisk       string
	hostname         string
	username         string
	timezone         string
	consoleKeymap    string
	chezmoiKeyLayout string
	machineName      string
	outputDir        string
	dryRun           bool
}

func defaultConfig() installConfig {
	return installConfig{
		hostname:         "rgx1gen11",
		username:         "rgoswami",
		timezone:         "America/Chicago",
		consoleKeymap:    "us",
		chezmoiKeyLayout: "colemak",
		machineName:      "rgx1gen11",
		outputDir:        "/run/hz-install",
	}
}

func main() {
	cfg, err := parseArgs(os.Args[1:], os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hz-install-tui: %v\n", err)
		os.Exit(2)
	}

	if cfg.dryRun {
		if err := validateConfig(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "hz-install-tui: %v\n", err)
			os.Exit(2)
		}
		if err := runBackend(cfg, true); err != nil {
			fmt.Fprintf(os.Stderr, "hz-install-tui: %v\n", err)
			os.Exit(1)
		}
		return
	}

	p := tea.NewProgram(newModel(cfg))
	final, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hz-install-tui: %v\n", err)
		os.Exit(1)
	}

	m, ok := final.(model)
	if !ok || m.action == actionNone {
		return
	}
	if err := validateConfig(m.cfg); err != nil {
		fmt.Fprintf(os.Stderr, "hz-install-tui: %v\n", err)
		os.Exit(2)
	}
	if err := runBackend(m.cfg, m.action == actionRender); err != nil {
		fmt.Fprintf(os.Stderr, "hz-install-tui: %v\n", err)
		os.Exit(1)
	}
}

func parseArgs(args []string, output io.Writer) (installConfig, error) {
	cfg := defaultConfig()
	fs := flag.NewFlagSet("hz-install-tui", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.BoolVar(&cfg.dryRun, "dry-run", false, "render archinstall JSON but do not install")
	fs.StringVar(&cfg.targetDisk, "target-disk", cfg.targetDisk, "whole disk to partition")
	fs.StringVar(&cfg.hostname, "hostname", cfg.hostname, "installed hostname")
	fs.StringVar(&cfg.username, "username", cfg.username, "primary sudo user")
	fs.StringVar(&cfg.timezone, "timezone", cfg.timezone, "installed timezone")
	fs.StringVar(&cfg.consoleKeymap, "console-keymap", cfg.consoleKeymap, "console keymap passed to archinstall")
	fs.StringVar(&cfg.chezmoiKeyLayout, "chezmoi-key-layout", cfg.chezmoiKeyLayout, "chezmoi key_layout value")
	fs.StringVar(&cfg.machineName, "machine-name", cfg.machineName, "chezmoi machine_name value")
	fs.StringVar(&cfg.outputDir, "output-dir", cfg.outputDir, "directory for rendered JSON")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if fs.NArg() != 0 {
		return cfg, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	return cfg, nil
}

func validateConfig(cfg installConfig) error {
	switch {
	case cfg.targetDisk == "":
		return errors.New("--target-disk is required")
	case !strings.HasPrefix(cfg.targetDisk, "/dev/"):
		return errors.New("--target-disk must be an absolute /dev path")
	case !hostnameRe.MatchString(cfg.hostname):
		return fmt.Errorf("invalid hostname: %s", cfg.hostname)
	case !usernameRe.MatchString(cfg.username):
		return fmt.Errorf("invalid username: %s", cfg.username)
	case strings.TrimSpace(cfg.timezone) == "":
		return errors.New("--timezone is required")
	case strings.TrimSpace(cfg.consoleKeymap) == "":
		return errors.New("--console-keymap is required")
	case strings.TrimSpace(cfg.chezmoiKeyLayout) == "":
		return errors.New("--chezmoi-key-layout is required")
	case strings.TrimSpace(cfg.machineName) == "":
		return errors.New("--machine-name is required")
	case strings.TrimSpace(cfg.outputDir) == "":
		return errors.New("--output-dir is required")
	default:
		return nil
	}
}

func runBackend(cfg installConfig, dryRun bool) error {
	backend := os.Getenv("HZ_INSTALL_BACKEND")
	if backend == "" {
		backend = defaultBackend
	}

	args := make([]string, 0, 18)
	if dryRun {
		args = append(args, "--dry-run")
	}
	args = append(args,
		"--target-disk", cfg.targetDisk,
		"--hostname", cfg.hostname,
		"--username", cfg.username,
		"--timezone", cfg.timezone,
		"--console-keymap", cfg.consoleKeymap,
		"--chezmoi-key-layout", cfg.chezmoiKeyLayout,
		"--machine-name", cfg.machineName,
		"--output-dir", cfg.outputDir,
	)

	cmd := exec.Command(backend, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type step int

const (
	stepWelcome step = iota
	stepInput
	stepReview
)

type action int

const (
	actionNone action = iota
	actionRender
	actionInstall
)

type field int

const (
	fieldDisk field = iota
	fieldHostname
	fieldUsername
	fieldTimezone
	fieldConsoleKeymap
	fieldChezmoiLayout
	fieldMachineName
	fieldOutputDir
	fieldCount
)

type fieldSpec struct {
	label       string
	help        string
	placeholder string
}

var fields = []fieldSpec{
	{label: "Target disk", help: "Whole disk path. This installer wipes it.", placeholder: "/dev/nvme0n1"},
	{label: "Hostname", help: "Installed system hostname.", placeholder: "rgx1gen11"},
	{label: "Username", help: "Primary sudo user.", placeholder: "rgoswami"},
	{label: "Timezone", help: "IANA timezone.", placeholder: "America/Chicago"},
	{label: "Console keymap", help: "Linux console keymap.", placeholder: "us"},
	{label: "Chezmoi layout", help: "Chezmoi key_layout value.", placeholder: "colemak"},
	{label: "Machine name", help: "Chezmoi machine_name value.", placeholder: "rgx1gen11"},
	{label: "Output dir", help: "Directory for archinstall JSON.", placeholder: "/run/hz-install"},
}

type model struct {
	cfg    installConfig
	step   step
	field  field
	input  textinput.Model
	action action
	err    string
}

func newModel(cfg installConfig) model {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.CharLimit = 128
	ti.SetWidth(52)
	ti.Focus()

	m := model{
		cfg:   cfg,
		step:  stepWelcome,
		field: fieldDisk,
		input: ti,
	}
	m.loadField()
	return m
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "q":
			if m.step != stepInput {
				return m, tea.Quit
			}
		case "enter":
			return m.handleEnter()
		case "tab":
			if m.step == stepInput {
				m.saveField()
				m.nextField()
				return m, nil
			}
		case "shift+tab":
			if m.step == stepInput {
				m.saveField()
				m.previousField()
				return m, nil
			}
		case "r":
			if m.step == stepReview {
				if err := validateConfig(m.cfg); err != nil {
					m.err = err.Error()
					return m, nil
				}
				m.action = actionRender
				return m, tea.Quit
			}
		case "i":
			if m.step == stepReview {
				if err := validateConfig(m.cfg); err != nil {
					m.err = err.Error()
					return m, nil
				}
				m.action = actionInstall
				return m, tea.Quit
			}
		case "e":
			if m.step == stepReview {
				m.step = stepInput
				m.field = fieldDisk
				m.loadField()
				return m, nil
			}
		}
	}

	if m.step != stepInput {
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.step {
	case stepWelcome:
		m.step = stepInput
	case stepInput:
		m.saveField()
		if m.field == fieldCount-1 {
			m.step = stepReview
			m.err = ""
			return m, nil
		}
		m.nextField()
	case stepReview:
		if err := validateConfig(m.cfg); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.action = actionRender
		return m, tea.Quit
	}
	return m, nil
}

func (m *model) loadField() {
	spec := fields[m.field]
	m.input.Placeholder = spec.placeholder
	m.input.SetValue(m.valueFor(m.field))
	m.input.Focus()
}

func (m *model) saveField() {
	value := strings.TrimSpace(m.input.Value())
	switch m.field {
	case fieldDisk:
		m.cfg.targetDisk = value
	case fieldHostname:
		m.cfg.hostname = value
	case fieldUsername:
		m.cfg.username = value
	case fieldTimezone:
		m.cfg.timezone = value
	case fieldConsoleKeymap:
		m.cfg.consoleKeymap = value
	case fieldChezmoiLayout:
		m.cfg.chezmoiKeyLayout = value
	case fieldMachineName:
		m.cfg.machineName = value
	case fieldOutputDir:
		m.cfg.outputDir = value
	}
}

func (m *model) nextField() {
	if m.field < fieldCount-1 {
		m.field++
	}
	m.loadField()
}

func (m *model) previousField() {
	if m.field > 0 {
		m.field--
	}
	m.loadField()
}

func (m model) valueFor(f field) string {
	switch f {
	case fieldDisk:
		return m.cfg.targetDisk
	case fieldHostname:
		return m.cfg.hostname
	case fieldUsername:
		return m.cfg.username
	case fieldTimezone:
		return m.cfg.timezone
	case fieldConsoleKeymap:
		return m.cfg.consoleKeymap
	case fieldChezmoiLayout:
		return m.cfg.chezmoiKeyLayout
	case fieldMachineName:
		return m.cfg.machineName
	case fieldOutputDir:
		return m.cfg.outputDir
	default:
		return ""
	}
}

func (m model) View() string {
	switch m.step {
	case stepWelcome:
		return panelStyle.Render(strings.Join([]string{
			titleStyle.Render("hzArchiso laptop installer"),
			"",
			"Encrypted Btrfs laptop profile with Sway, chezmoi, and Colemak defaults.",
			"",
			warnStyle.Render("This installer is destructive once you confirm a target disk."),
			"",
			"Enter  continue",
			"Esc    quit",
		}, "\n")) + "\n"
	case stepInput:
		spec := fields[m.field]
		return panelStyle.Render(strings.Join([]string{
			titleStyle.Render("Install choices"),
			progressLine(m.field),
			"",
			spec.label,
			mutedStyle.Render(spec.help),
			"",
			m.input.View(),
			"",
			"Enter  accept",
			"Tab    next",
			"Esc    quit",
		}, "\n")) + "\n"
	case stepReview:
		rows := []string{
			titleStyle.Render("Review install plan"),
			"",
			kv("Disk", m.cfg.targetDisk),
			kv("Hostname", m.cfg.hostname),
			kv("User", m.cfg.username),
			kv("Timezone", m.cfg.timezone),
			kv("Console keymap", m.cfg.consoleKeymap),
			kv("Chezmoi layout", m.cfg.chezmoiKeyLayout),
			kv("Machine name", m.cfg.machineName),
			kv("Output dir", m.cfg.outputDir),
			kv("Filesystem", "LUKS + Btrfs subvolumes"),
			kv("Kernels", "linux, linux-lts"),
			kv("Desktop", "Sway + Waybar + PipeWire"),
			"",
			warnStyle.Render("Install mode asks the backend for exact disk and password confirmation."),
			"",
			"r/Enter  render JSON",
			"i        install",
			"e        edit",
			"Esc      quit",
		}
		if m.err != "" {
			rows = append(rows[:2], append([]string{warnStyle.Render(m.err), ""}, rows[2:]...)...)
		}
		return panelStyle.Render(strings.Join(rows, "\n")) + "\n"
	default:
		return ""
	}
}

func progressLine(current field) string {
	parts := make([]string, 0, len(fields))
	for i, spec := range fields {
		name := spec.label
		if len(name) > 12 {
			name = name[:12]
		}
		if field(i) == current {
			parts = append(parts, titleStyle.Render(name))
		} else {
			parts = append(parts, mutedStyle.Render(name))
		}
	}
	return strings.Join(parts, "  ")
}

func kv(label, value string) string {
	if strings.TrimSpace(value) == "" {
		value = "<unset>"
	}
	return fmt.Sprintf("%-15s %s", label+":", value)
}
