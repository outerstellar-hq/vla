package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/abrandt/vla/internal/agent"
	"github.com/abrandt/vla/internal/app"
	"github.com/abrandt/vla/internal/config"
	"github.com/abrandt/vla/internal/indexer"
	"github.com/abrandt/vla/internal/lsp"
	"github.com/abrandt/vla/internal/mcp"
	"github.com/abrandt/vla/internal/session"
	"github.com/abrandt/vla/internal/tools"
	"github.com/abrandt/vla/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
)

// isInteractive returns true if stdin is a TTY (and thus the full-screen TUI
// should be used). When input is piped, we fall back to readline/plain mode.
func isInteractive() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
}

// runTUI starts the full-screen bubbletea interface. The agent loop runs in
// a background goroutine, communicating with the TUI via channels.
//
// The session switcher: when the user picks a session in the TUI picker,
// a session ID is sent on switchCh. The runner closes the current input
// (causing loop.Run to return), opens the new session, loads its messages,
// and restarts the loop — all without killing bubbletea.
func runTUI(
	loop *agent.Loop,
	cfg *config.Config,
	reg *tools.Registry,
	sess *session.Session,
	watcher *indexer.Watcher,
	lspMgr *lsp.Manager,
	mcpMgr *mcp.Manager,
	autoApprove bool,
	sessionIdx *session.Index,
	planMode bool,
	persona string,
) {
	currentSess := sess

	// Session switch channel: TUI sends session IDs here.
	switchCh := make(chan string, 1)

	// Session lister for the picker: filters by current project.
	sessionLister := func(project string) []tui.SessionItem {
		var entries []session.IndexEntry
		if project != "" {
			entries = sessionIdx.ListByProject(project)
		} else {
			entries = sessionIdx.List()
		}
		items := make([]tui.SessionItem, len(entries))
		for i, e := range entries {
			items[i] = tui.SessionItem{
				ID:      e.ID,
				Project: e.Project,
				Model:   e.Model,
				Created: e.Created,
			}
		}
		return items
	}

	for {
		// Channels between TUI and agent loop for this iteration.
		inputCh := tui.NewChannelInput()
		streamWriter := tui.NewChannelWriter()
		eventCh := make(chan agent.Event, 64)
		cancelCh := make(chan struct{})

		// Wire the loop for the current session.
		loop.SetInput(inputCh)
		loop.SetEventChan(eventCh)
		loop.SetTranscriptWriter(currentSess.Append)
		loop.SetCancelChannel(cancelCh)

		// Load messages for this session (fixes resume-into-TUI bug).
		systemMsg := buildSystemMsg(planMode, persona, currentSess.CWD())
		if hasHistory(currentSess) {
			msgs, err := app.LoadTranscriptMessages(currentSess)
			if err == nil && len(msgs) > 0 {
				loop.LoadMessages(append([]agent.Message{systemMsg}, msgs...))
			} else {
				loop.LoadMessages([]agent.Message{systemMsg})
			}
		} else {
			loop.LoadMessages([]agent.Message{systemMsg})
		}

		// TUI-native approval.
		var approver *tui.TUIApprover
		if !autoApprove {
			approver = tui.NewTUIApprover()
			loop.SetApprover(approver)
		}

		// Create the TUI model.
		model := tui.New(
			cfg.Model,
			len(reg.Schemas()),
			currentSess.ID(),
			inputCh.Ch,
			streamWriter.Chan(),
			eventCh,
			approver,
			switchCh,
			cancelCh,
			sessionLister,
			currentSess.CWD(),
		)
		model.SetContextLimit(cfg.ContextLimit)

		// Start bubbletea in a goroutine.
		p := tea.NewProgram(model, tea.WithAltScreen())
		go func() {
			if _, err := p.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "vla: tui error: %v\n", err)
			}
		}()

		// Run the agent loop (blocking). Output goes to streamWriter.
		// When the user picks a new session, switchCh fires; we close
		// inputCh to make loop.Run return, then loop around.
		loopDone := make(chan error, 1)
		go func() {
			loopDone <- loop.Run(nil, streamWriter)
		}()

		select {
		case <-loopDone:
			// Normal exit (Ctrl+C or EOF) — clean up and return.
			inputCh.Close()
			watcher.Stop()
			lspMgr.Close()
			mcpMgr.Close()
			return

		case newID := <-switchCh:
			// Session switch: tear down current loop, open new session.
			inputCh.Close() // causes loop.Run to return (EOF on input)

			// Open the new session.
			newSess, err := openSessionByID(newID, cfg.Model)
			if err != nil {
				fmt.Fprintf(os.Stderr, "vla: could not open session %s: %v\n", newID, err)
				continue // stay on current session
			}

			// Record in index.
			sessionIdx.Record(newSess.ID(), newSess.CWD(), cfg.Model)
			currentSess = newSess
			// Loop around to restart with the new session.
		}
	}
}

// buildSystemMsg returns the system prompt message (plan mode or persona-based).
func buildSystemMsg(planMode bool, persona, projectDir string) agent.Message {
	promptText := resolvePersona(persona, projectDir)
	if planMode {
		promptText = app.PlanModePrompt()
	}
	return agent.Message{Role: agent.RoleSystem, Content: promptText}
}

// hasHistory returns true if the session has a non-empty transcript.
func hasHistory(sess *session.Session) bool {
	turns, _, err := sess.Read()
	if err != nil {
		return false
	}
	return len(turns) > 0
}

// openSessionByID opens a session by its ID from ~/.vla/sessions/<id>.json.
func openSessionByID(id, model string) (*session.Session, error) {
	path := filepath.Join(session.SessionsDir(), id+".json")
	return session.Open(path)
}
