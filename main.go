// VLA — Very Large Agent.
// A CLI agentic coding harness. See docs/DESIGN.md.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/abrandt/vla/internal/agent"
	"github.com/abrandt/vla/internal/app"
	"github.com/abrandt/vla/internal/compaction"
	"github.com/abrandt/vla/internal/config"
	"github.com/abrandt/vla/internal/llm"
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

	reg := tools.NewRegistry()
	if err := app.RegisterBuiltins(reg, sess.CWD()); err != nil {
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
