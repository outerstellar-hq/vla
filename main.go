// VLA — Very Large Agent.
// A CLI agentic coding harness. See docs/DESIGN.md.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/abrandt/vla/internal/agent"
	"github.com/abrandt/vla/internal/compaction"
	"github.com/abrandt/vla/internal/config"
	"github.com/abrandt/vla/internal/llm"
	"github.com/abrandt/vla/internal/session"
	"github.com/abrandt/vla/internal/tools"
	"github.com/abrandt/vla/internal/tools/builtin"
)

func main() {
	resume := flag.String("resume", "", "session ID to resume (default: new session)")
	modelFlag := flag.String("model", "", "override config model for this run")
	configFlag := flag.String("config", "", "path to config.json (default: ./config.json then ~/.vla/config.json)")
	flag.Parse()

	cfgPath := resolveConfigPath(*configFlag)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vla: load config: %v\n", err)
		os.Exit(1)
	}
	if *modelFlag != "" {
		cfg.Model = *modelFlag
	}

	sess, err := openOrCreateSession(*resume, cfg.Model)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vla: session: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "vla: session %s (cwd %s)\n", sess.ID(), sess.CWD())

	// For a resumed session, restore the working directory it was created in.
	if *resume != "" {
		if err := os.Chdir(sess.CWD()); err != nil {
			fmt.Fprintf(os.Stderr, "vla: warn: could not chdir to %s: %v\n", sess.CWD(), err)
		}
	}

	reg := tools.NewRegistry()
	if err := registerBuiltins(reg); err != nil {
		fmt.Fprintf(os.Stderr, "vla: register tools: %v\n", err)
		os.Exit(1)
	}

	client := llm.NewClient(cfg.APIKey, cfg.BaseURL, cfg.Model)
	summarizer := newSummarizer(client)
	loop := agent.NewLoop(client, reg, compaction.Compact, summarizer, compaction.CharThreshold)

	if err := loop.Run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "vla: %v\n", err)
		os.Exit(1)
	}
}

// resolveConfigPath finds config.json: explicit flag → CWD → ~/.vla/config.json.
func resolveConfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if _, err := os.Stat("config.json"); err == nil {
		return "config.json"
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".vla", "config.json")
	}
	return "config.json" // let Load report the error
}

// openOrCreateSession opens an existing session (--resume) or creates a new one.
func openOrCreateSession(resumeID, model string) (*session.Session, error) {
	if resumeID != "" {
		path := filepath.Join(session.SessionsDir(), resumeID+".json")
		return session.Open(path)
	}
	return session.New(session.WithModel(model))
}

// registerBuiltins adds all built-in tools to the registry.
// To add a tool: implement tools.Tool in its own file, then add one line here.
func registerBuiltins(r *tools.Registry) error {
	builtins := []tools.Tool{
		builtin.Echo{},
	}
	for _, t := range builtins {
		if err := r.Register(t); err != nil {
			return err
		}
	}
	return nil
}

// newSummarizer returns the production Summarizer: it calls the LLM to
// produce a terse summary of older conversation turns for compaction.
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
