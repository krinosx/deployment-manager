package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/krinosx/deployment-manager/internal/history"
)

// model holds all the state for our TUI. Right now it's just a placeholder.
type model struct {
	message string
}

// Init runs once when the program starts. It can return a command to run
// (e.g. "load data"), or nil if there's nothing to do yet.
func (m model) Init() tea.Cmd {
	return nil
}

// Update is called every time something happens (a key press, a timer, data
// loading finishing, etc). It receives the event and returns the new state
// plus any follow-up command.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}
	return m, nil
}

// View renders the current state as a string. Called after every Update.
func (m model) View() string {
	return m.message + "\n\nPress q to quit.\n"
}

func main() {

	entries, err := history.FetchEntries()

	if err != nil {
		fmt.Fprintln(os.Stderr, "error fetching deploy logs:", err)
		os.Exit(1)
	}

	fmt.Printf("Fetched %d entries\n", len(entries))

	runs := history.GroupByRun(entries)
	fmt.Printf("Grouped %d runs\n", len(runs))

	for _, run := range runs {
		fmt.Printf("Run %s (sha=%s)\n", run.RunID, run.SHA)
		for _, stage := range run.Stages {
			fmt.Printf("  - stage=%-10s status=%-8s duration_ms=%s \n", stage.Stage, stage.Status, stage.DurationMs)
		}
		fmt.Println()
	}

	initialModel := model{message: "deploy-tui — hello world"}

	p := tea.NewProgram(initialModel)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error running TUI:", err)
		os.Exit(1)
	}
}
