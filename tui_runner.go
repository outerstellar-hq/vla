package main

import (
	"fmt"
	"os"

	"github.com/abrandt/vla/internal/agent"
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
// a background goroutine, communicating with the TUI via channels:
//   - inputCh:     TUI → loop (user-submitted text)
//   - streamWriter: loop → TUI (raw streaming tokens via io.Writer)
//   - eventCh:     loop → TUI (typed events: tool calls, usage, turn boundaries)
func runTUI(
	loop *agent.Loop,
	cfg *config.Config,
	reg *tools.Registry,
	sess *session.Session,
	watcher *indexer.Watcher,
	lspMgr *lsp.Manager,
	mcpMgr *mcp.Manager,
	autoApprove bool,
) {
	// Channels between TUI and agent loop.
	inputCh := tui.NewChannelInput()       // TUI → loop: user messages
	streamWriter := tui.NewChannelWriter() // loop → TUI: streaming tokens
	eventCh := make(chan agent.Event, 64)  // loop → TUI: typed events (buffered)

	// Wire the loop to use channel input, stream output, and event channel.
	loop.SetInput(inputCh)
	loop.SetEventChan(eventCh)

	// TUI-native approval: only if the loop has an approver set (i.e. --yes
	// was not passed). The TUIApprover routes y/n/a prompts through the TUI
	// instead of ReadlineApprover (which deadlocks in alt-screen mode).
	var approver *tui.TUIApprover
	if !autoApprove {
		approver = tui.NewTUIApprover()
		loop.SetApprover(approver)
	}

	// Create the TUI model with the new signature.
	model := tui.New(
		cfg.Model,
		len(reg.Schemas()),
		sess.ID(),
		inputCh.Ch,
		streamWriter.Chan(),
		eventCh,
		approver,
	)

	// Start bubbletea in a goroutine so we can run the agent loop on the main
	// goroutine (the loop blocks on input from the channel).
	p := tea.NewProgram(model, tea.WithAltScreen())
	go func() {
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "vla: tui error: %v\n", err)
		}
	}()

	// Run the agent loop (blocking). Output goes to streamWriter so the TUI
	// can display streaming tokens. Input comes from inputCh (the TUI).
	if err := loop.Run(nil, streamWriter); err != nil {
		fmt.Fprintf(streamWriter, "Error: %v\n", err)
	}

	// Cleanup.
	inputCh.Close()
	watcher.Stop()
	lspMgr.Close()
	mcpMgr.Close()
}
