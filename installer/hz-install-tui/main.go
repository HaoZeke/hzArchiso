package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	defaultBackend   = "/usr/local/bin/hz-install"
	profileRGX1      = "rgx1gen11"
	profileAM5Terra  = "rgam5terra"
	profileRGSURFLat = "rgSURFLat"
)

// profileInfo is one supported machine profile (TUI menu + CLI help).
type profileInfo struct {
	slug    string
	summary string
}

// supportedProfiles is the single catalog for validation, menus, and help text.
func supportedProfiles() []profileInfo {
	return []profileInfo{
		{slug: profileRGX1, summary: "ThinkPad X1 Carbon Gen 11 laptop (default)"},
		{slug: profileRGSURFLat, summary: "Dell Latitude 7430 Intel laptop (ucode, mesa, SOF, TLP)"},
		{slug: profileAM5Terra, summary: "AM5 Terra desktop (amd-ucode, NVIDIA open, tuned)"},
	}
}

func profilesHelpText() string {
	parts := make([]string, 0, len(supportedProfiles()))
	for i, p := range supportedProfiles() {
		parts = append(parts, fmt.Sprintf("%d=%s", i+1, p.slug))
	}
	return strings.Join(parts, ", ")
}

func profileSummary(slug string) string {
	for _, p := range supportedProfiles() {
		if p.slug == slug {
			return p.summary
		}
	}
	return ""
}

// resolveProfileInput accepts a slug or a 1-based menu index from the catalog.
func resolveProfileInput(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("profile is required")
	}
	profiles := supportedProfiles()
	if n, err := strconv.Atoi(raw); err == nil {
		if n < 1 || n > len(profiles) {
			return "", fmt.Errorf("profile index %d out of range (1-%d: %s)", n, len(profiles), profilesHelpText())
		}
		return profiles[n-1].slug, nil
	}
	for _, p := range profiles {
		if p.slug == raw {
			return p.slug, nil
		}
	}
	return "", fmt.Errorf("unsupported profile %q (choose %s)", raw, profilesHelpText())
}

func profileMenuLines() []string {
	lines := make([]string, 0, len(supportedProfiles())+1)
	lines = append(lines, mutedStyle.Render("Profiles (type number or slug):"))
	for i, p := range supportedProfiles() {
		lines = append(lines, fmt.Sprintf("  %d) %-12s  %s", i+1, p.slug, mutedStyle.Render(p.summary)))
	}
	return lines
}

// keysFooter renders consistent navigation hints for the active step.
func keysFooter(step step) string {
	switch step {
	case stepWelcome:
		return mutedStyle.Render("keys: Enter continue · Esc quit")
	case stepInput:
		return mutedStyle.Render("keys: Enter accept field · Tab next · Shift-Tab prev · Esc quit")
	case stepReview:
		return mutedStyle.Render("keys: r/Enter dry-run plan · i install · e edit · Esc quit")
	case stepInstallConfirm:
		return mutedStyle.Render("keys: type exact disk path · Enter install · Esc quit")
	default:
		return ""
	}
}

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
	errStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#DC2626"))
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#00A884")).
			Padding(1, 2).
			Width(78)
)

type installConfig struct {
	targetDisk       string
	profile          string
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
		profile:          profileRGX1,
		hostname:         profileRGX1,
		username:         "rgoswami",
		timezone:         "America/Chicago",
		consoleKeymap:    "us",
		chezmoiKeyLayout: "colemak",
		machineName:      profileRGX1,
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
	fs.BoolVar(&cfg.dryRun, "dry-run", false, "print the install plan but do not install")
	fs.StringVar(&cfg.profile, "profile", cfg.profile, "machine profile slug or 1-based index ("+profilesHelpText()+")")
	fs.StringVar(&cfg.targetDisk, "target-disk", cfg.targetDisk, "whole disk to partition")
	fs.StringVar(&cfg.hostname, "hostname", cfg.hostname, "installed hostname")
	fs.StringVar(&cfg.username, "username", cfg.username, "primary sudo user")
	fs.StringVar(&cfg.timezone, "timezone", cfg.timezone, "installed timezone")
	fs.StringVar(&cfg.consoleKeymap, "console-keymap", cfg.consoleKeymap, "console keymap")
	fs.StringVar(&cfg.chezmoiKeyLayout, "chezmoi-key-layout", cfg.chezmoiKeyLayout, "chezmoi key_layout value")
	fs.StringVar(&cfg.machineName, "machine-name", cfg.machineName, "chezmoi machine_name value")
	fs.StringVar(&cfg.outputDir, "output-dir", cfg.outputDir, "directory for install plan output")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if fs.NArg() != 0 {
		return cfg, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	seen := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) {
		seen[f.Name] = true
	})
	if seen["profile"] {
		resolved, err := resolveProfileInput(cfg.profile)
		if err != nil {
			return cfg, err
		}
		cfg.profile = resolved
	}
	if err := applyProfileDefaults(&cfg, !seen["hostname"], !seen["machine-name"]); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func validateConfig(cfg installConfig) error {
	switch {
	case !validProfile(cfg.profile):
		return fmt.Errorf("unsupported profile: %s", cfg.profile)
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

func validProfile(profile string) bool {
	_, err := resolveProfileInput(profile)
	return err == nil
}

func profileDefaultName(profile string) (string, bool) {
	slug, err := resolveProfileInput(profile)
	if err != nil {
		return "", false
	}
	return slug, true
}

// validateField checks one interactive field before leaving it.
func validateField(f field, value string) error {
	value = strings.TrimSpace(value)
	switch f {
	case fieldProfile:
		_, err := resolveProfileInput(value)
		return err
	case fieldDisk:
		if value == "" {
			return errors.New("target disk is required (whole disk, e.g. /dev/nvme0n1)")
		}
		if !strings.HasPrefix(value, "/dev/") {
			return errors.New("target disk must be an absolute /dev path")
		}
		return nil
	case fieldHostname:
		if !hostnameRe.MatchString(value) {
			return fmt.Errorf("invalid hostname: %s", value)
		}
		return nil
	case fieldUsername:
		if !usernameRe.MatchString(value) {
			return fmt.Errorf("invalid username: %s", value)
		}
		return nil
	case fieldTimezone:
		if value == "" {
			return errors.New("timezone is required (IANA, e.g. America/Chicago)")
		}
		return nil
	case fieldConsoleKeymap:
		if value == "" {
			return errors.New("console keymap is required")
		}
		return nil
	case fieldChezmoiLayout:
		if value == "" {
			return errors.New("chezmoi key layout is required")
		}
		return nil
	case fieldMachineName:
		if value == "" {
			return errors.New("machine name is required")
		}
		return nil
	case fieldOutputDir:
		if value == "" {
			return errors.New("output dir is required")
		}
		return nil
	default:
		return nil
	}
}

func applyProfileDefaults(cfg *installConfig, setHostname, setMachineName bool) error {
	defaultName, ok := profileDefaultName(cfg.profile)
	if !ok {
		return fmt.Errorf("unsupported profile: %s", cfg.profile)
	}
	if setHostname {
		cfg.hostname = defaultName
	}
	if setMachineName {
		cfg.machineName = defaultName
	}
	return nil
}

func runBackend(cfg installConfig, dryRun bool) error {
	backend := os.Getenv("HZ_INSTALL_BACKEND")
	if backend == "" {
		backend = defaultBackend
	}

	args := make([]string, 0, 20)
	if dryRun {
		args = append(args, "--dry-run")
	}
	args = append(args,
		"--profile", cfg.profile,
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
	stepInstallConfirm
)

type action int

const (
	actionNone action = iota
	actionRender
	actionInstall
)

type field int

const (
	fieldProfile field = iota
	fieldDisk
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
	{label: "Profile", help: "Hardware package set and hostname defaults. Pick a number from the list or type the slug.", placeholder: "1, 2, 3, or slug"},
	{label: "Target disk", help: "Whole disk path that will be wiped (ESP + LUKS root).", placeholder: "/dev/nvme0n1"},
	{label: "Hostname", help: "Installed system hostname (defaults from profile).", placeholder: profileRGX1},
	{label: "Username", help: "Primary sudo user on the installed system.", placeholder: "rgoswami"},
	{label: "Timezone", help: "IANA timezone for timedatectl.", placeholder: "America/Chicago"},
	{label: "Console keymap", help: "Linux console keymap (loadkeys).", placeholder: "us"},
	{label: "Chezmoi layout", help: "Chezmoi data.key_layout (keyboard layout policy).", placeholder: "colemak"},
	{label: "Machine name", help: "Chezmoi data.machine_name (host policy selector).", placeholder: profileRGX1},
	{label: "Output dir", help: "Directory for install-plan.txt and related artifacts.", placeholder: "/run/hz-install"},
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
				if err := m.trySaveField(); err != nil {
					m.err = err.Error()
					return m, nil
				}
				m.err = ""
				m.nextField()
				return m, nil
			}
		case "shift+tab":
			if m.step == stepInput {
				// Allow moving back even if current field is incomplete.
				_ = m.trySaveFieldOptional()
				m.err = ""
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
				m.startInstallConfirmation()
				return m, nil
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

	if m.step != stepInput && m.step != stepInstallConfirm {
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
		m.err = ""
	case stepInput:
		if err := m.trySaveField(); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.err = ""
		if m.field == fieldCount-1 {
			if err := validateConfig(m.cfg); err != nil {
				m.err = err.Error()
				return m, nil
			}
			m.step = stepReview
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
	case stepInstallConfirm:
		confirmation := strings.TrimSpace(m.input.Value())
		if confirmation != m.cfg.targetDisk {
			m.err = "confirmation did not match target disk — type the exact path shown above"
			m.input.SetValue("")
			return m, nil
		}
		m.action = actionInstall
		return m, tea.Quit
	}
	return m, nil
}

func (m *model) startInstallConfirmation() {
	m.step = stepInstallConfirm
	m.err = ""
	m.input.Placeholder = m.cfg.targetDisk
	m.input.SetValue("")
	m.input.Focus()
}

func (m *model) loadField() {
	spec := fields[m.field]
	m.input.Placeholder = spec.placeholder
	m.input.SetValue(m.valueFor(m.field))
	m.input.Focus()
}

// trySaveField validates and commits the current input field.
func (m *model) trySaveField() error {
	value := strings.TrimSpace(m.input.Value())
	if err := validateField(m.field, value); err != nil {
		return err
	}
	m.commitField(value)
	return nil
}

// trySaveFieldOptional commits only when validation passes (used for Shift-Tab).
func (m *model) trySaveFieldOptional() error {
	value := strings.TrimSpace(m.input.Value())
	if err := validateField(m.field, value); err != nil {
		return err
	}
	m.commitField(value)
	return nil
}

func (m *model) commitField(value string) {
	switch m.field {
	case fieldProfile:
		oldProfile := m.cfg.profile
		slug, err := resolveProfileInput(value)
		if err != nil {
			return
		}
		m.cfg.profile = slug
		m.applyInteractiveProfileDefaults(oldProfile)
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

// saveField is retained for tests that set the input and commit without validation.
func (m *model) saveField() {
	value := strings.TrimSpace(m.input.Value())
	if m.field == fieldProfile {
		if slug, err := resolveProfileInput(value); err == nil {
			value = slug
		}
	}
	m.commitField(value)
}

func (m *model) applyInteractiveProfileDefaults(oldProfile string) {
	newDefault, ok := profileDefaultName(m.cfg.profile)
	if !ok {
		return
	}
	oldDefault, ok := profileDefaultName(oldProfile)
	if !ok {
		oldDefault = ""
	}
	if m.cfg.hostname == "" || m.cfg.hostname == oldDefault {
		m.cfg.hostname = newDefault
	}
	if m.cfg.machineName == "" || m.cfg.machineName == oldDefault {
		m.cfg.machineName = newDefault
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
	case fieldProfile:
		return m.cfg.profile
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

func (m model) View() tea.View {
	switch m.step {
	case stepWelcome:
		rows := []string{
			titleStyle.Render("hzArchiso machine installer"),
			"",
			"Native Go installer: LUKS2 + Btrfs subvolumes + systemd-boot.",
			"Desktop defaults: Sway, chezmoi, Colemak — profile selects hardware packages.",
			"",
		}
		rows = append(rows, profileMenuLines()...)
		rows = append(rows, "",
			warnStyle.Render("Destructive once you confirm the exact target disk path."),
			"",
			keysFooter(stepWelcome),
		)
		return tea.NewView(panelStyle.Render(strings.Join(rows, "\n")) + "\n")
	case stepInput:
		spec := fields[m.field]
		rows := []string{
			titleStyle.Render("Install choices"),
			progressLine(m.field),
			"",
			titleStyle.Render(spec.label),
			mutedStyle.Render(spec.help),
			"",
		}
		if m.field == fieldProfile {
			rows = append(rows, profileMenuLines()...)
			rows = append(rows, "")
		}
		if m.field == fieldDisk {
			rows = append(rows, warnStyle.Render("Warning: the selected disk will be fully repartitioned."), "")
		}
		rows = append(rows, m.input.View(), "")
		if m.err != "" {
			rows = append(rows, errStyle.Render("✗ "+m.err), "")
		}
		rows = append(rows, keysFooter(stepInput))
		return tea.NewView(panelStyle.Render(strings.Join(rows, "\n")) + "\n")
	case stepReview:
		summary := profileSummary(m.cfg.profile)
		rows := []string{
			titleStyle.Render("Review install plan"),
			"",
			kv("Profile", m.cfg.profile),
		}
		if summary != "" {
			rows = append(rows, mutedStyle.Render("  "+summary))
		}
		rows = append(rows,
			kv("Disk", m.cfg.targetDisk),
			kv("Hostname", m.cfg.hostname),
			kv("User", m.cfg.username),
			kv("Timezone", m.cfg.timezone),
			kv("Console keymap", m.cfg.consoleKeymap),
			kv("Chezmoi layout", m.cfg.chezmoiKeyLayout),
			kv("Machine name", m.cfg.machineName),
			kv("Output dir", m.cfg.outputDir),
			kv("Filesystem", "LUKS2 + Btrfs (@ @home @snapshots …)"),
			kv("Kernels", "linux, linux-lts"),
			kv("Desktop", "Sway + Waybar + PipeWire"),
			"",
			warnStyle.Render("Install runs the native Go backend (sgdisk, LUKS, pacstrap)."),
			warnStyle.Render("You must re-type the exact disk path before anything is wiped."),
			"",
			keysFooter(stepReview),
		)
		if m.err != "" {
			rows = append(rows[:2], append([]string{errStyle.Render("✗ " + m.err), ""}, rows[2:]...)...)
		}
		return tea.NewView(panelStyle.Render(strings.Join(rows, "\n")) + "\n")
	case stepInstallConfirm:
		rows := []string{
			titleStyle.Render("Confirm destructive install"),
			"",
			warnStyle.Render("This will erase the selected disk. Passwords are collected next by the backend."),
			"",
			kv("Profile", m.cfg.profile),
			kv("Disk", m.cfg.targetDisk),
			kv("Hostname", m.cfg.hostname),
			"",
			"Type the exact disk path to continue:",
			m.input.View(),
			"",
		}
		if m.err != "" {
			rows = append(rows, errStyle.Render("✗ "+m.err), "")
		}
		rows = append(rows, keysFooter(stepInstallConfirm))
		return tea.NewView(panelStyle.Render(strings.Join(rows, "\n")) + "\n")
	default:
		return tea.NewView("")
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
