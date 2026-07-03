// VLA — Very Large Agent.
// A CLI agentic coding harness. See docs/DESIGN.md.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/abrandt/vla/internal/agent"
	"github.com/abrandt/vla/internal/app"
	"github.com/abrandt/vla/internal/approval"
	"github.com/abrandt/vla/internal/commands"
	"github.com/abrandt/vla/internal/compaction"
	"github.com/abrandt/vla/internal/config"
	"github.com/abrandt/vla/internal/indexer"
	"github.com/abrandt/vla/internal/llm"
	"github.com/abrandt/vla/internal/lsp"
	"github.com/abrandt/vla/internal/mcp"
	"github.com/abrandt/vla/internal/memory"
	"github.com/abrandt/vla/internal/modelsdev"
	"github.com/abrandt/vla/internal/permissions"
	"github.com/abrandt/vla/internal/tools"
	"github.com/abrandt/vla/internal/tools/builtin"
)

func main() {
	// Subcommand routing: "vla models", "vla use <provider/model>", or default (agent loop).
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "models":
			runModelsCmd(os.Args[2:])
			return
		case "use":
			runUseCmd(os.Args[2:])
			return
		case "version":
			fmt.Println("vla dev")
			return
		}
	}
	runAgent()
}

// runAgent is the main agent loop (default when no subcommand is given).
func runAgent() {
	resume := flag.String("resume", "", "session ID to resume (default: new session)")
	modelFlag := flag.String("model", "", "override config model for this run")
	configFlag := flag.String("config", "", "path to config.json (default: ./config.json then ~/.vla/config.json)")
	yesFlag := flag.Bool("yes", false, "auto-approve all tool calls (no confirmation prompts)")
	planFlag := flag.Bool("plan", false, "plan mode: read-only investigation, no file modifications")
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

	// Start MCP servers (external tools from .vla/mcp.json).
	mcpCfg, err := mcp.LoadConfig(baseDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vla: warn: mcp config: %v\n", err)
		mcpCfg = &mcp.ConfigFile{Servers: map[string]mcp.ServerConfig{}}
	}
	mcpMgr := mcp.NewManager()
	mcpMgr.StartAll(mcpCfg, func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, format, args...)
	})
	defer mcpMgr.Close()

	// Set up memory store + embeddings.
	memStore := memory.NewStore(memory.DefaultRoot())
	var embedder *memory.EmbeddingClient
	if cfg.APIKey != "" {
		embedder = memory.NewEmbeddingClient(cfg.APIKey, cfg.BaseURL, "")
	}

	projectName := func() string { return baseDir }
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

	// Register MCP tools (external tools from .vla/mcp.json).
	if err := mcp.RegisterAll(reg, mcpMgr); err != nil {
		fmt.Fprintf(os.Stderr, "vla: warn: mcp tool registration: %v\n", err)
	}
	fmt.Fprintf(os.Stderr, "vla: %d tools registered\n", len(reg.Schemas()))

	client := llm.NewClient(cfg.APIKey, cfg.BaseURL, cfg.Model)
	summarizer := newSummarizer(client)
	// Compaction threshold: 75% of the model's context window (in tokens).
	// Falls back to DefaultTokenThreshold if context_limit isn't set.
	threshold := compaction.DefaultTokenThreshold
	if cfg.ContextLimit > 0 {
		threshold = cfg.ContextLimit * 3 / 4
	}
	loop := agent.NewLoop(client, reg, compaction.Compact, summarizer, threshold)
	loop.SetContextInjector(injector)
	loop.SetTranscriptWriter(sess.Append)

	// Permission system: load .vla/permissions.json (deny rules block tools
	// before they reach the approver).
	permMgr, err := permissions.Load(baseDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vla: warn: permissions: %v\n", err)
		permMgr = &permissions.Manager{Default: permissions.ActionAllow}
	}
	// In plan mode, deny all destructive tools — the LLM can investigate
	// but not modify anything.
	if *planFlag {
		for _, tool := range []string{"write_file", "update_file", "delete_file", "git_commit"} {
			permMgr.AddOverride(tool, permissions.ActionDeny)
		}
		fmt.Fprintf(os.Stderr, "vla: plan mode — file modifications blocked\n")
	}
	loop.SetPermissionChecker(permChecker{permMgr})

	// Approval system: --yes flag skips all prompts; otherwise prompt before
	// destructive tools (write_file, update_file, delete_file, git_commit).
	if *yesFlag {
		loop.SetApprover(alwaysApprover{})
	} else if isInteractive() {
		loop.SetApprover(approverAdapter{approval.NewReadlineApprover()})
	}

	// Slash commands: /help, /tools, /memory, /compact, /session
	loop.SetCommandHandler(func(input string) (string, bool) {
		result := commands.Execute(input, commands.Context{
			Registry:  reg,
			Model:     cfg.Model,
			SessionID: sess.ID(),
			ToolCount: len(reg.Schemas()),
			MemSearch: func(q string) (string, error) {
				raw, _ := json.Marshal(map[string]string{"query": q})
				return builtin.MemorySearch{Deps: builtin.MemoryTools{
					Store: memStore, Project: projectName,
				}}.Execute(raw)
			},
			MemSave: func(text string) (string, error) {
				raw, _ := json.Marshal(map[string]string{"content": text})
				return builtin.MemorySave{Deps: builtin.MemoryTools{
					Store: memStore, Project: projectName,
				}}.Execute(raw)
			},
			GetUsage: func() (int, int, int) {
				u := client.TotalUsage()
				return u.PromptTokens, u.CompletionTokens, u.TotalTokens
			},
			GetCost: func() float64 {
				u := client.TotalUsage()
				return float64(u.PromptTokens)*2.5/1e6 + float64(u.CompletionTokens)*10.0/1e6
			},
		})
		return result.Output, result.Handled
	})

	// Use the TUI for interactive terminals; fall back to readline for piped
	// input or when the terminal doesn't support raw mode.
	if isInteractive() {
		runTUI(loop, cfg, reg, sess, watcher, lspMgr, mcpMgr)
		return
	}

	// Fallback: readline mode.
	rl, err := newReadline()
	if err != nil {
		fmt.Fprintf(os.Stderr, "vla: warn: readline unavailable (%v), using plain input\n", err)
	} else {
		defer rl.Close()
		loop.SetInput(rl)
	}

	// On resume, reload prior messages and prepend the system prompt.
	promptText := app.SystemPrompt()
	if *planFlag {
		promptText = app.PlanModePrompt()
	}
	systemMsg := agent.Message{Role: agent.RoleSystem, Content: promptText}
	if *resume != "" {
		msgs, err := app.LoadTranscriptMessages(sess)
		if err != nil {
			fmt.Fprintf(os.Stderr, "vla: warn: could not load transcript: %v\n", err)
		} else {
			loop.LoadMessages(append([]agent.Message{systemMsg}, msgs...))
			fmt.Fprintf(os.Stderr, "vla: resumed %d messages\n", len(msgs))
		}
	} else {
		loop.LoadMessages([]agent.Message{systemMsg})
	}

	// Catch Ctrl+C for clean shutdown.
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

// runModelsCmd handles `vla models [provider] [filter]`.
func runModelsCmd(args []string) {
	client := modelsdev.NewClient(modelsdev.DefaultCacheDir())
	providers, err := client.Fetch()
	if err != nil {
		fmt.Fprintf(os.Stderr, "vla: could not fetch model catalog: %v\n", err)
		os.Exit(1)
	}
	switch len(args) {
	case 0:
		modelsdev.PrintProviders(providers, "")
	case 1:
		//vla models <provider>
		modelsdev.PrintModels(args[0], providers, "")
	case 2:
		//vla models <provider> <filter>
		modelsdev.PrintModels(args[0], providers, args[1])
	default:
		fmt.Fprintln(os.Stderr, "usage: vla models [provider] [filter]")
		os.Exit(1)
	}
}

// runUseCmd handles `vla use <provider/model>`.
// It resolves the provider+model from models.dev, finds the API key from the
// environment, and writes a config.json so the agent loop picks it up next run.
func runUseCmd(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: vla use <provider/model>")
		fmt.Fprintln(os.Stderr, "example: vla use openai/gpt-4o")
		fmt.Fprintln(os.Stderr, "         vla use anthropic/claude-sonnet-4-5")
		os.Exit(1)
	}
	spec := args[0]

	client := modelsdev.NewClient(modelsdev.DefaultCacheDir())
	providers, err := client.Fetch()
	if err != nil {
		fmt.Fprintf(os.Stderr, "vla: could not fetch model catalog: %v\n", err)
		os.Exit(1)
	}

	sel, err := modelsdev.Select(providers, spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vla: %v\n", err)
		os.Exit(1)
	}

	if sel.APIKey == "" {
		fmt.Fprintf(os.Stderr, "vla: no API key found for %s.\n", sel.Provider.Name)
		if len(sel.Provider.Env) > 0 {
			fmt.Fprintf(os.Stderr, "Set one of these environment variables:\n")
			for _, envVar := range sel.Provider.Env {
				fmt.Fprintf(os.Stderr, "  export %s=your-key-here\n", envVar)
			}
		}
		os.Exit(1)
	}

	if sel.BaseURL == "" || sel.BaseURL == "none" {
		fmt.Fprintf(os.Stderr, "vla: %s has no OpenAI-compatible API URL in the catalog\n", sel.Provider.Name)
		os.Exit(1)
	}

	// Write config.json in CWD.
	cfg := struct {
		APIKey       string `json:"api_key"`
		BaseURL      string `json:"base_url"`
		Model        string `json:"model"`
		ContextLimit int    `json:"context_limit,omitempty"`
	}{
		APIKey:       sel.APIKey,
		BaseURL:      sel.BaseURL,
		Model:        sel.ModelID,
		ContextLimit: sel.Model.Limit.Context,
	}
	data, _ := jsonMarshal(cfg)
	if err := writeFile("config.json", data); err != nil {
		fmt.Fprintf(os.Stderr, "vla: write config.json: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ config.json written for %s %s (%s)\n", sel.Provider.Name, sel.Model.Name, sel.ModelID)
	if sel.Model.Limit.Context > 0 {
		fmt.Printf("  context window: %d tokens\n", sel.Model.Limit.Context)
	}
	if sel.Model.ToolCall {
		fmt.Printf("  tool calling: supported\n")
	}
	fmt.Printf("  api: %s\n", sel.BaseURL)
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
		resp, err := client.StreamTo(summaryReq, nil, nil)
		if err != nil {
			return "", err
		}
		return resp.Content, nil
	}
}

func jsonMarshal(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}
