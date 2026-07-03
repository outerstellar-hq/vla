// VLA — Very Large Agent.
// A CLI agentic coding harness. See docs/DESIGN.md.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
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
	"github.com/abrandt/vla/internal/session"
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
	injector := newMemoryInjector(memStore, embedder, projectName)

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

	// On resume, reload prior messages from the transcript.
	if *resume != "" {
		msgs, err := loadTranscriptMessages(sess)
		if err != nil {
			fmt.Fprintf(os.Stderr, "vla: warn: could not load transcript: %v\n", err)
		} else if len(msgs) > 0 {
			loop.LoadMessages(msgs)
			fmt.Fprintf(os.Stderr, "vla: resumed %d messages\n", len(msgs))
		}
	} else {
		// New session: inject a system prompt as the first message so the
		// LLM knows what it is and what tools it has.
		loop.LoadMessages([]agent.Message{{
			Role:    agent.RoleSystem,
			Content: systemPrompt(),
		}})
	}

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

// loadTranscriptMessages reads the transcript NDJSON and converts turns back
// into agent.Message objects for session resume.
func loadTranscriptMessages(sess *session.Session) ([]agent.Message, error) {
	turns, _, err := sess.Read()
	if err != nil {
		return nil, err
	}
	var msgs []agent.Message
	for _, t := range turns {
		roleStr, _ := t["role"].(string)
		if roleStr == "" {
			continue
		}
		msg := agent.Message{Role: agent.Role(roleStr)}
		msg.Content, _ = t["content"].(string)
		msg.ToolCallID, _ = t["tool_call_id"].(string)
		// tool_calls round-trip: stored as []any, need to convert back.
		if tcs, ok := t["tool_calls"].([]any); ok {
			for _, tc := range tcs {
				tcMap, ok := tc.(map[string]any)
				if !ok {
					continue
				}
				var call agent.ToolCall
				call.ID, _ = tcMap["id"].(string)
				call.Type, _ = tcMap["type"].(string)
				if fn, ok := tcMap["function"].(map[string]any); ok {
					call.Function.Name, _ = fn["name"].(string)
					call.Function.Arguments, _ = fn["arguments"].(string)
				}
				msg.ToolCalls = append(msg.ToolCalls, call)
			}
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

// systemPrompt returns the system message that tells the LLM what VLA is and
// how to use its tools. This is the difference between an LLM that flails and
// one that navigates code like a developer.
func systemPrompt() string {
	return `You are VLA (Very Large Agent), an agentic coding harness. You operate directly on the user's codebase via tools.

You have these tools available:
- File: read_file, write_file, update_file, delete_file, list_files
- Search: search (text search across the codebase)
- Git: git_status, git_diff, git_commit
- Navigation: go_to_definition, find_references, hover, diagnostics
- Memory: memory_save, memory_search, memory_list, memory_delete
- Web: web_search, web_read

When investigating a task:
1. Start by listing files or searching to understand the codebase structure.
2. Read relevant files before making changes.
3. Use update_file for targeted edits (provide unique old_string). Use write_file only for new files.
4. After changes, check git_diff to verify what changed.
5. Use memory_save to persist important findings, decisions, or architecture notes for future sessions. Use memory_search to recall them.
6. Use go_to_definition and find_references to understand how code connects — like ctrl+click in an IDE.

Be concise. Don't explain what you're about to do — just do it, then report the result.`
}

// newMemoryInjector creates a context injector that searches memories relevant
// to the current user message and prepends them as a system message.
func newMemoryInjector(store *memory.Store, embedder *memory.EmbeddingClient, project func() string) agent.ContextInjector {
	return func(view []agent.Message, lastUserMessage string) []agent.Message {
		if lastUserMessage == "" {
			return view
		}
		var queryVec []float32
		if embedder != nil {
			queryVec, _ = embedder.Embed(lastUserMessage)
		}
		results, err := store.Search(project(), lastUserMessage, queryVec, 5, 0.7, 0.3)
		if err != nil || len(results) == 0 {
			return view
		}
		var b strings.Builder
		b.WriteString("Relevant memories from previous sessions:\n\n")
		for _, r := range results {
			fmt.Fprintf(&b, "- %s\n", r.Memory.Content)
		}
		b.WriteString("\nUse these memories if relevant. Ignore if not.\n")
		injected := append([]agent.Message{{Role: agent.RoleSystem, Content: b.String()}}, view...)
		return injected
	}
}
