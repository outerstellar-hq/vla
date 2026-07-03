package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/abrandt/vla/internal/agent"
	"github.com/chzyer/readline"
)

// newReadline creates a readline instance for the VLA REPL with:
// - A custom prompt
// - History persistence (~/.vla/history)
// - Multi-line input support via backslash continuation
// - Proper Ctrl+C handling (cancels line, doesn't kill process)
func newReadline() (*readlineInstance, error) {
	historyPath := historyPath()
	rl, err := readline.NewEx(&readline.Config{
		Prompt:            "\033[31m»\033[0m ",
		HistoryFile:       historyPath,
		AutoComplete:      nil, // could add tab completion later
		InterruptPrompt:   "^C\n",
		EOFPrompt:         "exit\n",
		HistorySearchFold: true,
	})
	if err != nil {
		return nil, fmt.Errorf("readline: %w", err)
	}
	return &readlineInstance{rl: rl}, nil
}

// readlineInstance adapts *readline.Instance to agent.InputReader.
type readlineInstance struct {
	rl *readline.Instance
}

func (r *readlineInstance) Readline() (string, error) {
	line, err := r.rl.Readline()
	if err == readline.ErrInterrupt {
		// Ctrl+C cancels the current line, not the process.
		return "", nil
	}
	return line, err
}

func (r *readlineInstance) Close() error {
	return r.rl.Close()
}

// historyPath returns ~/.vla/history, creating the dir if needed.
func historyPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".vla")
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "history")
}

// Compile-time check that readlineInstance satisfies agent.InputReader.
var _ agent.InputReader = (*readlineInstance)(nil)
