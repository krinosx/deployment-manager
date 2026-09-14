package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/krinosx/deployment-manager/internal/history"
)

const deployCheckUnit = "deploy-check.service"

var (
	passStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	failStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("237")).Bold(true)
	headerStyle   = lipgloss.NewStyle().Bold(true).Underline(true)
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

// historyLoadedMsg carries the result of loading and grouping journal
// entries, fed back into Update() once the async load completes.
type historyLoadedMsg struct {
	runs []history.Run
	err  error
}

// triggerDoneMsg carries the result of a "trigger now" request. systemctl
// start on a oneshot unit blocks until the run finishes, so by the time this
// arrives the new run's journal entries already exist.
type triggerDoneMsg struct {
	output string
	err    error
}

// loadHistory is a tea.Cmd: it runs the journalctl subprocess off the UI
// thread and returns the runs newest-first.
func loadHistory() tea.Msg {
	entries, err := history.FetchEntries()
	if err != nil {
		return historyLoadedMsg{err: err}
	}

	runs := history.GroupByRun(entries)
	for i, j := 0, len(runs)-1; i < j; i, j = i+1, j-1 {
		runs[i], runs[j] = runs[j], runs[i]
	}

	return historyLoadedMsg{runs: runs}
}

// triggerDeploy runs deploy-check now via a scoped, passwordless sudo rule
// (see docs/AI-Handover/05-deploy-tui.md) since deploy-check.service is a
// system-wide unit and deploy-tui runs as a regular user.
func triggerDeploy() tea.Msg {
	out, err := exec.Command("sudo", "systemctl", "start", deployCheckUnit).CombinedOutput()
	return triggerDoneMsg{output: string(out), err: err}
}

type viewMode int

const (
	viewList viewMode = iota
	viewDetail
)

type model struct {
	runs       []history.Run
	cursor     int
	loading    bool
	err        error
	triggering bool
	statusMsg  string
	mode       viewMode

	// pendingTriggerRefresh and preTriggerTopRunID let the post-trigger
	// refresh tell a no-op deploy-check run apart from a real run being
	// added.
	pendingTriggerRefresh bool
	preTriggerTopRunID    string

	// showNoOp controls whether NO-OP runs (checked, nothing new to
	// deploy) are included in the visible list. Toggled with 'n'.
	showNoOp bool
}

// visibleRuns returns m.runs filtered by the current showNoOp setting. The
// cursor and all list/detail rendering index into this, not m.runs
// directly, so the displayed list and what "enter" opens always agree.
func (m model) visibleRuns() []history.Run {
	if m.showNoOp {
		return m.runs
	}
	visible := make([]history.Run, 0, len(m.runs))
	for _, r := range m.runs {
		if r.OverallStatus() != "NO-OP" {
			visible = append(visible, r)
		}
	}
	return visible
}

func initialModel() model {
	return model{loading: true, showNoOp: true}
}

func (m model) Init() tea.Cmd {
	return loadHistory
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case historyLoadedMsg:
		m.loading = false
		m.err = msg.err
		m.runs = msg.runs
		if m.cursor >= len(m.visibleRuns()) {
			m.cursor = 0
		}
		if len(m.visibleRuns()) == 0 {
			m.mode = viewList
		}
		if m.pendingTriggerRefresh {
			m.pendingTriggerRefresh = false
			if msg.err == nil {
				newTopRunID := ""
				if len(m.runs) > 0 {
					newTopRunID = m.runs[0].RunID
				}
				if newTopRunID != "" && newTopRunID != m.preTriggerTopRunID {
					m.statusMsg = "deploy-check ran: a new run was recorded below."
				} else {
					m.statusMsg = "deploy-check ran but found no new commits to deploy (nothing to show — remote is already at the last-deployed SHA)."
				}
			}
		}
	case triggerDoneMsg:
		m.triggering = false
		if msg.err != nil {
			m.pendingTriggerRefresh = false
			m.statusMsg = fmt.Sprintf("trigger failed: %v: %s", msg.err, strings.TrimSpace(msg.output))
			return m, nil
		}
		m.statusMsg = "deploy-check finished, checking for a new run..."
		m.loading = true
		return m, loadHistory
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		switch m.mode {
		case viewDetail:
			if msg.String() == "esc" {
				m.mode = viewList
			}
		default:
			switch msg.String() {
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "j":
				if m.cursor < len(m.visibleRuns())-1 {
					m.cursor++
				}
			case "enter":
				if len(m.visibleRuns()) > 0 {
					m.mode = viewDetail
				}
			case "n":
				m.showNoOp = !m.showNoOp
				if m.cursor >= len(m.visibleRuns()) {
					m.cursor = 0
				}
			case "r":
				if !m.loading && !m.triggering {
					m.loading = true
					m.statusMsg = ""
					return m, loadHistory
				}
			case "t":
				if !m.triggering {
					m.triggering = true
					m.statusMsg = "triggering deploy-check now..."
					m.pendingTriggerRefresh = true
					m.preTriggerTopRunID = ""
					if len(m.runs) > 0 {
						m.preTriggerTopRunID = m.runs[0].RunID
					}
					return m, triggerDeploy
				}
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.loading {
		return "Loading deploy history...\n"
	}
	if m.err != nil {
		return errStyle.Render(fmt.Sprintf("error fetching deploy logs: %v", m.err)) + "\n"
	}

	if m.mode == viewDetail {
		return m.renderDetail()
	}

	var status string
	if m.triggering {
		status = helpStyle.Render("triggering deploy-check now, this may take a while (build in progress)...") + "\n\n"
	} else if m.statusMsg != "" {
		status = helpStyle.Render(m.statusMsg) + "\n\n"
	}

	visible := m.visibleRuns()

	if len(visible) == 0 {
		empty := "No deploy runs found."
		if len(m.runs) > 0 {
			empty = "All runs are NO-OP and are currently hidden. Press n to show them."
		}
		return status + empty + "\n\n" + helpStyle.Render("n: toggle no-op • t: trigger now • q: quit") + "\n"
	}

	var b strings.Builder
	b.WriteString(status)
	b.WriteString(headerStyle.Render(fmt.Sprintf("  %-9s %-6s %s", "SHA", "STATUS", "TIME")))
	b.WriteString("\n")

	for i, run := range visible {
		sha := run.SHA
		if len(sha) > 9 {
			sha = sha[:9]
		}
		status := run.OverallStatus()
		ts := run.Timestamp().Format("2006-01-02 15:04:05")
		plain := fmt.Sprintf("%-9s %-6s %s", sha, status, ts)

		if i == m.cursor {
			b.WriteString(selectedStyle.Render("> " + plain))
		} else {
			statusStyle := lipgloss.NewStyle()
			switch status {
			case "PASS":
				statusStyle = passStyle
			case "FAIL":
				statusStyle = failStyle
			}
			b.WriteString(fmt.Sprintf("  %-9s %s %s", sha, statusStyle.Render(fmt.Sprintf("%-6s", status)), ts))
		}
		b.WriteString("\n")
	}

	noOpLabel := "hide no-op"
	if !m.showNoOp {
		noOpLabel = "show no-op"
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render(fmt.Sprintf("↑/↓: navigate • enter: view logs • n: %s • t: trigger now • r: refresh • q: quit", noOpLabel)))
	b.WriteString("\n")

	return b.String()
}

// renderDetail shows the selected run's per-stage breakdown, including each
// stage's captured command output (DEPLOY_OUTPUT).
func (m model) renderDetail() string {
	run := m.visibleRuns()[m.cursor]

	overall := run.OverallStatus()
	overallStyle := lipgloss.NewStyle()
	switch overall {
	case "PASS":
		overallStyle = passStyle
	case "FAIL":
		overallStyle = failStyle
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render(fmt.Sprintf("Run %s", run.RunID)))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("SHA: %s    Status: %s    Time: %s\n\n",
		run.SHA, overallStyle.Render(overall), run.Timestamp().Format("2006-01-02 15:04:05")))

	for _, s := range run.Stages {
		stageStyle := lipgloss.NewStyle()
		switch s.Status {
		case "success":
			stageStyle = passStyle
		case "failure":
			stageStyle = failStyle
		}

		label := headerStyle.Render(fmt.Sprintf("[%s]", s.Stage))
		b.WriteString(fmt.Sprintf("%s %s (%sms)\n", label, stageStyle.Render(s.Status), s.DurationMs))

		output := strings.TrimSpace(s.Output)
		if output == "" {
			output = "(no output captured)"
		}
		b.WriteString(output)
		b.WriteString("\n\n")
	}

	b.WriteString(helpStyle.Render("esc: back to list • q: quit"))
	b.WriteString("\n")

	return b.String()
}

func main() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error running TUI:", err)
		os.Exit(1)
	}
}
