// VLA — Very Large Agent.
// A CLI agentic coding harness. See docs/DESIGN.md.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/abrandt/vla/internal/agent"
	"github.com/abrandt/vla/internal/app"
	"github.com/abrandt/vla/internal/compaction"
	"github.com/abrandt/vla/internal/config"
	"github.com/abrandt/vla/internal/indexer"
	"github.com/abrandt/vla/internal/llm"
	"github.com/abrandt/vla/internal/lsp"
	"github.com/abrandt/vla/internal/memory"
	"github.com/abrandt/vla/internal/tools"
)

func main() {
	resume := flag.String("resume", "", "session ID to resume (default: new session)")
	modelFlag := flag.String("model", "", "override config model for this run")
	configFlag := flag.String("config", "", "path to config.json (default: ./config.json then ~/.vla/config.json)")
	flag.Parse()

	cfgPath := app.ResolveConfigPath(*configFlag)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vla: load config: %v\n", err)
		os.Exit(1)
	}
	if *modelFlag != "" {
		cfg.Model = *modelFlag
	}

	sess, err := app.OpenOrCreateSession(*resume, cfg.Model)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vla: session: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "vla: session %s (cwd %s)\n", sess.ID(), sess.CWD())

	if *resume != "" {
		if err := os.Chdir(sess.CWD()); err != nil {
			fmt.Fprintf(os.Stderr, "vla: warn: could not chdir to %s: %v\n", sess.CWD(), err)
		}
	}

	// Start the background indexer (regex-based symbol index).
	baseDir := sess.CWD()
	ix := indexer.New(baseDir)
	if n, err := ix.Build(); err != nil {
		fmt.Fprintf(os.Stderr, "vla: warn: initial index build failed: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "vla: indexed %d files\n", n)
	}
	watcher := indexer.NewWatcher(ix, 5*time.Second)
	watcher.Start()
	defer watcher.Stop()

	// Start the LSP manager (for real go-to-def, hover, diagnostics).
	lspMgr := lsp.NewManager(lsp.DefaultSpecs())
	defer lspMgr.Close()

	// Set up memory store + embeddings.
	memStore := memory.NewStore(memory.DefaultRoot())
	var embedder *memory.EmbeddingClient
	// Only enable embeddings if we have an API key.
	if cfg.APIKey != "" {
		embedder = memory.NewEmbeddingClient(cfg.APIKey, cfg.BaseURL, "")
	}

	projectName := func() string { return baseDir }

	// Create the memory context injector — auto-injects relevant memories
	// before each LLM call.
	injector := app.NewMemoryInjector(memStore, embedder, projectName)

	reg := tools.NewRegistry()
	if err := app.RegisterBuiltins(reg, app.Deps{
		BaseDir:    baseDir,
		Indexer:    ix,
		LSPManager: lspMgr,
		MemStore:   memStore,
		Embedder:   embedder,
		Project:    projectName,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "vla: register tools: %v\n", err)
		os.Exit(1)
	}

	client := llm.NewClient(cfg.APIKey, cfg.BaseURL, cfg.Model)
	summarizer := newSummarizer(client)
	loop := agent.NewLoop(client, reg, compaction.Compact, summarizer, compaction.CharThreshold)
	loop.SetContextInjector(injector)
	loop.SetTranscriptWriter(sess.Append)

	// On resume, reload prior messages from the transcript and prepend the
	// system prompt so the LLM still knows what it is and what tools it has.
	// (Without this, a resumed session has no system message at all.)
	systemMsg := agent.Message{Role: agent.RoleSystem, Content: app.SystemPrompt()}
	if *resume != "" {
		msgs, err := app.LoadTranscriptMessages(sess)
		if err != nil {
			fmt.Fprintf(os.Stderr, "vla: warn: could not load transcript: %v\n", err)
		} else {
			// Prepend system prompt to the resumed conversation.
			loop.LoadMessages(append([]agent.Message{systemMsg}, msgs...))
			fmt.Fprintf(os.Stderr, "vla: resumed %d messages\n", len(msgs))
		}
	} else {
		loop.LoadMessages([]agent.Message{systemMsg})
	}

	// Catch Ctrl+C for clean shutdown: stop the watcher, kill LSP servers,
	// flush the transcript. Without this, orphan processes leak.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		<-sigChan
		fmt.Fprintf(os.Stderr, "\nvla: interrupt received, shutting down...\n")
		watcher.Stop()
		lspMgr.Close()
		os.Exit(0)
	}()

	if err := loop.Run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "vla: %v\n", err)
		os.Exit(1)
	}
}

// newSummarizer returns the production Summarizer for compaction.
func newSummarizer(client *llm.Client) agent.Summarizer {
	return func(msgs []agent.Message) (string, error) {
		var b strings.Builder
		for _, m := range msgs {
			fmt.Fprintf(&b, "[%s] %s\n", m.Role, m.Content)
		}
		summaryReq := []agent.Message{{
			Role: agent.RoleUser,
			Content: "Summarize the following conversation turns. Preserve: file paths mentioned, " +
				"decisions made, errors encountered, and any incomplete tasks. Be terse.\n\n" + b.String(),
		}}
		resp, err := client.StreamTo(summaryReq, nil, io.Discard)
		if err != nil {
			return "", err
		}
		return resp.Content, nil
	}
}
