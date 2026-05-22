package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("8"))
	successStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	failStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	stageStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	envStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
	envProdStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9"))
)

type deviceRow struct {
	bay        int
	serial     string
	deviceID   string
	stage      FWStage
	stageStart time.Time
	endTime    time.Time
	target     string
	reason     string
	applyWait  time.Duration
	finished   bool
	succeeded  bool
}

// FWModel is the Bubble Tea model for the fw-update TUI.
type FWModel struct {
	devices       map[int]*deviceRow
	bayOrder      []int
	spinner       spinner.Model
	startTime     time.Time
	doneAt        time.Time
	width, height int
	env           string
	firmwareDesc  string
	allDone       bool
	quitRequested bool
}

// NewFWModel builds the model. `bays` must contain every bay number that will
// receive events. `env` is the target environment (e.g. "dev", "prod") shown
// in the header. `firmwareDesc` is a one-line summary of what is being flashed.
func NewFWModel(bays []int, env string, firmwareDesc string) FWModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))

	sorted := append([]int(nil), bays...)
	sort.Ints(sorted)

	rows := make(map[int]*deviceRow, len(sorted))
	for _, b := range sorted {
		rows[b] = &deviceRow{bay: b, stage: FWStageIdentifying, stageStart: time.Now()}
	}
	return FWModel{
		devices:      rows,
		bayOrder:     sorted,
		spinner:      s,
		startTime:    time.Now(),
		env:          env,
		firmwareDesc: firmwareDesc,
	}
}

// FWEventMsg wraps a FWProgressEvent so workers can push updates via
// (*tea.Program).Send.
type FWEventMsg FWProgressEvent

// FWAllDoneMsg signals that every worker has finished. The TUI redraws once
// more and then quits on the next keypress.
type FWAllDoneMsg struct{}

// NewFWProgressFunc returns a FWProgressFunc that pushes events into the
// running tea.Program. Safe to call from worker goroutines.
func NewFWProgressFunc(p *tea.Program) FWProgressFunc {
	return func(ev FWProgressEvent) {
		p.Send(FWEventMsg(ev))
	}
}

type fwTickMsg time.Time

func fwTick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return fwTickMsg(t)
	})
}

// Init starts the spinner and a redraw tick.
func (m FWModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, fwTick())
}

// Update handles incoming messages.
func (m FWModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitRequested = true
			return m, tea.Quit
		case "q":
			if m.allDone {
				return m, tea.Quit
			}
		}
		return m, nil

	case fwTickMsg:
		return m, fwTick()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case FWEventMsg:
		m.applyEvent(FWProgressEvent(msg))
		return m, nil

	case FWAllDoneMsg:
		m.allDone = true
		if m.doneAt.IsZero() {
			m.doneAt = time.Now()
		}
		return m, nil
	}
	return m, nil
}

func (m *FWModel) applyEvent(ev FWProgressEvent) {
	r, ok := m.devices[ev.Bay]
	if !ok {
		return
	}
	if ev.DeviceID != "" {
		r.deviceID = ev.DeviceID
	}
	if ev.Serial != "" {
		r.serial = ev.Serial
	}
	if ev.Target != "" {
		r.target = ev.Target
	}
	if ev.Reason != "" {
		r.reason = ev.Reason
	}
	if ev.Wait != 0 {
		r.applyWait = ev.Wait
	}
	r.stage = ev.Stage
	r.stageStart = time.Now()
	switch ev.Stage {
	case FWStageCompleted:
		r.finished = true
		r.succeeded = true
		if r.endTime.IsZero() {
			r.endTime = time.Now()
		}
	case FWStageFailed:
		r.finished = true
		if r.endTime.IsZero() {
			r.endTime = time.Now()
		}
	}
}

// View renders the current model state.
func (m FWModel) View() string {
	var b strings.Builder

	// Title row.
	envLabel := m.env
	envRendered := envStyle.Render(envLabel)
	if strings.EqualFold(envLabel, "prod") {
		envRendered = envProdStyle.Render(envLabel)
	}
	fmt.Fprintf(&b, "%s on %s — %s\n",
		titleStyle.Render(fmt.Sprintf("fw-update · %d device(s)", len(m.bayOrder))),
		envRendered,
		dimStyle.Render(m.firmwareDesc),
	)
	b.WriteString(dimStyle.Render(strings.Repeat("─", minInt(80, maxInt(40, m.width-1)))))
	b.WriteString("\n")

	// Column header.
	fmt.Fprintf(&b, "%s  %s  %s  %s\n",
		headerStyle.Render(padRight("Bay", 4)),
		headerStyle.Render(padRight("Serial", 12)),
		headerStyle.Render(padRight("Stage", 40)),
		headerStyle.Render("Elapsed"),
	)

	// One row per device.
	const stageWidth = 40
	for _, bay := range m.bayOrder {
		r := m.devices[bay]
		serial := r.serial
		if serial == "" {
			serial = "—"
		}
		stageText := ansiFit(m.renderStage(r), stageWidth)
		var elapsed time.Duration
		if r.finished {
			elapsed = r.endTime.Sub(m.startTime).Truncate(time.Second)
		} else {
			elapsed = time.Since(r.stageStart).Truncate(time.Second)
		}
		fmt.Fprintf(&b, "%s  %s  %s  %s\n",
			padRight(fmt.Sprintf("%d", r.bay), 4),
			padRight(truncate(serial, 12), 12),
			stageText,
			dimStyle.Render(elapsed.String()),
		)
	}

	// Footer.
	b.WriteString(dimStyle.Render(strings.Repeat("─", minInt(80, maxInt(40, m.width-1)))))
	b.WriteString("\n")

	done, ok, fail, running := m.counts()
	overall := time.Since(m.startTime).Truncate(time.Second)
	if !m.doneAt.IsZero() {
		overall = m.doneAt.Sub(m.startTime).Truncate(time.Second)
	}
	fmt.Fprintf(&b, " %s · %s · %s · %s   %s\n",
		dimStyle.Render(fmt.Sprintf("%d running", running)),
		successStyle.Render(fmt.Sprintf("%d ok", ok)),
		failStyle.Render(fmt.Sprintf("%d failed", fail)),
		dimStyle.Render(fmt.Sprintf("%d/%d done", done, len(m.bayOrder))),
		dimStyle.Render(fmt.Sprintf("elapsed %s", overall)),
	)
	if m.allDone {
		b.WriteString("\n")
		b.WriteString(m.renderConclusion(ok, fail, overall))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(" press q to exit\n"))
	} else {
		b.WriteString(dimStyle.Render(" press ctrl+c to quit (workers keep running)\n"))
	}
	return b.String()
}

func (m FWModel) renderConclusion(ok, fail int, elapsed time.Duration) string {
	total := len(m.bayOrder)

	var headline string
	switch {
	case fail == 0:
		headline = successStyle.Render(fmt.Sprintf("✓ All %d device(s) updated", total)) +
			"  " + dimStyle.Render("· "+elapsed.String())
	case ok == 0:
		headline = failStyle.Render(fmt.Sprintf("✗ All %d device(s) failed", total)) +
			"  " + dimStyle.Render("· "+elapsed.String())
	default:
		warn := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
		headline = warn.Render(fmt.Sprintf("⚠ %d/%d updated, %d failed", ok, total, fail)) +
			"  " + dimStyle.Render("· "+elapsed.String())
	}

	body := headline
	if fail > 0 {
		body += "\n\n" + headerStyle.Render("Failures")
		for _, bay := range m.bayOrder {
			r := m.devices[bay]
			if !r.finished || r.succeeded {
				continue
			}
			reason := r.reason
			if reason == "" {
				reason = "unknown"
			}
			body += fmt.Sprintf("\n  bay %d: %s", r.bay, truncate(reason, 64))
		}
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")).
		Padding(0, 1)
	return boxStyle.Render(body)
}

func (m FWModel) renderStage(r *deviceRow) string {
	switch r.stage {
	case FWStageIdentifying:
		return m.spinner.View() + " " + stageStyle.Render("identifying")
	case FWStageIdentified:
		return m.spinner.View() + " " + stageStyle.Render("identified")
	case FWStagePoweringOff:
		return m.spinner.View() + " " + stageStyle.Render("powering off")
	case FWStageWriting:
		t := r.target
		if t == "" {
			t = "firmware"
		}
		return m.spinner.View() + " " + stageStyle.Render("writing "+shortTarget(t))
	case FWStageRebooting:
		return m.spinner.View() + " " + stageStyle.Render("rebooting")
	case FWStageApplying:
		remaining := r.applyWait - time.Since(r.stageStart)
		if remaining < 0 {
			remaining = 0
		}
		return m.spinner.View() + " " + stageStyle.Render(
			fmt.Sprintf("applying (%s left)", remaining.Truncate(time.Second)),
		)
	case FWStageVerifying:
		return m.spinner.View() + " " + stageStyle.Render("verifying")
	case FWStageCompleted:
		return successStyle.Render("✓ completed")
	case FWStageFailed:
		msg := "✗ failed"
		if r.reason != "" {
			msg = fmt.Sprintf("✗ failed: %s", r.reason)
		}
		return failStyle.Render(msg)
	}
	return ""
}

func (m FWModel) counts() (done, ok, fail, running int) {
	for _, r := range m.devices {
		if r.finished {
			done++
			if r.succeeded {
				ok++
			} else {
				fail++
			}
		} else {
			running++
		}
	}
	return
}

// shortTarget converts internal target keys like "$firmware_path" into a
// human label.
func shortTarget(t string) string {
	switch t {
	case "$firmware_path":
		return "main"
	case "$bw16_firmware_path":
		return "wifi"
	case "$pm_firmware_path":
		return "pmm"
	}
	return t
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

// ansiFit truncates a possibly-ANSI-styled string with "…" and pads it on
// the right so the visible width is exactly n.
func ansiFit(s string, n int) string {
	visible := lipgloss.Width(s)
	if visible > n {
		s = ansi.Truncate(s, n, "…")
		visible = lipgloss.Width(s)
	}
	if visible < n {
		s += strings.Repeat(" ", n-visible)
	}
	return s
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
