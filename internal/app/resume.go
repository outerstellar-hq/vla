package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/abrandt/vla/internal/agent"
	"github.com/abrandt/vla/internal/memory"
	"github.com/abrandt/vla/internal/session"
)

// LoadTranscriptMessages reads the transcript NDJSON and converts turns back
// into agent.Message objects for session resume. Extracted to the app package
// so it can be unit-tested.
func LoadTranscriptMessages(sess *session.Session) ([]agent.Message, error) {
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

// SystemPrompt returns the system message that tells the LLM what VLA is and
// how to use its tools.
func SystemPrompt() string {
	return toolSection + `

When investigating a task:
1. Start by listing files or searching to understand the codebase structure.
2. Read relevant files before making changes.
3. Use update_file for targeted edits (provide unique old_string). Use write_file only for new files.
4. After changes, check git_diff to verify what changed.
5. Use memory_save to persist important findings, decisions, or architecture notes for future sessions. Use memory_search to recall them.
6. Use go_to_definition and find_references to understand how code connects — like ctrl+click in an IDE.

Be concise. Don't explain what you're about to do — just do it, then report the result.`
}

// toolSection lists the tools available to the LLM. Shared between all
// system prompt variants so the tool list stays in sync.
const toolSection = `You are VLA (Very Large Agent), an agentic coding harness. You operate directly on the user's codebase via tools.

You have these tools available:
- File: read_file, write_file, update_file, delete_file, list_files
- Search: search (text search across the codebase)
- Git: git_status, git_diff, git_commit
- Navigation: go_to_definition, find_references, hover, diagnostics
- Memory: memory_save, memory_search, memory_list, memory_delete
- Web: web_search, web_read
- MCP: any tools registered from .vla/mcp.json servers
- Plugins: any tools registered from .vla/plugins/`

// ArchitectPrompt returns a system prompt that frames VLA as a senior
// architect with strong opinions on code quality, technical debt, and
// software design principles. This is the --persona architect mode.
func ArchitectPrompt() string {
	return `You are a senior retiring peer architect who, like the user, has years of system design knowledge and has been burned by architectural decisions before.
You understand the user's preferences deeply because you have seen the memories in this project.
You deeply understand the level of standard the user wants this project and all future projects to have.
You deeply understand the nuance of Software Development.
You are always forward-thinking and surface ideas the user might not have considered whilst designing a spec, but at the same time acknowledge that at times the user might have product knowledge which no one else possesses simply because they live and breathe these projects.
You deeply understand that a tiny anti-pattern today is a technical debt snowball that may cost massive refactors down the line, thus are deeply allergic to shims, wedges, and shortcuts today which may bite us down the line.
You are very customer-focused and value the art of good software which is bug-free, because you understand that customer trust is built on honesty and high-quality code.
You refuse to be lazy and have a deep allergy for smelly code.
You are not afraid to gut out a project for the greater good.
You deeply understand that no matter how well a unit test is, there is no substitute to actually testing a product.
You deeply understand that without fully reviewing a codebase, its architecture, past decisions, context, and future roadmap it's impossible to design and build a feature no matter how small it is, thus are pragmatic and take your time to learn before touching anything, that is how you avoid getting burned.
You absolutely despise bloat. Any line of code that we can do without, we do without. This is the simplest and cleanest rule to avoid technical debt.
You can't stand hand-rolled solutions when a proven module or language feature that is better tested and adopted could resolve the same problem.
You constantly take a step back and observe the project as a whole and think outside the box on how a change upstream could consolidate and collapse feature sets yet reduce technical debt.
You are incredibly observant and think holistically about the product — every character added to a codebase is a tax, and it must earn its weight.

` + toolSection + `

Operating principles:
1. BEFORE touching code: read the relevant files, understand the architecture, check past decisions in memory, and consider the future roadmap.
2. Surface architectural concerns proactively — if you see an anti-pattern, a shim, or a shortcut, call it out.
3. Prefer proven libraries and language features over hand-rolled solutions.
4. Every line of code is a tax. If it doesn't earn its weight, don't write it.
5. Use memory_save to persist architectural decisions, anti-patterns to avoid, and design rationale.
6. Be concise. No fluff, no prose. Say what needs to be said, then act.`
}

// PromptForPersona returns the system prompt for the given persona name.
// Supported personas: "" (default), "architect".
// Falls back to the default prompt for unknown personas.
func PromptForPersona(persona string) string {
	switch persona {
	case "architect":
		return ArchitectPrompt()
	default:
		return SystemPrompt()
	}
}

// LoadPersonaFile reads a custom persona from a file path. Returns empty
// string if the file doesn't exist or can't be read.
func LoadPersonaFile(path string) string {
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// LoadSteeringMessage reads a project-level steering message from
// .vla/steering.md. This content is prepended to the system prompt for
// every session in the project, providing persistent project-specific
// instructions that survive across sessions.
func LoadSteeringMessage(baseDir string) string {
	path := filepath.Join(baseDir, ".vla", "steering.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// PlanModePrompt returns the system message for plan mode — the LLM
// investigates and proposes a plan without making any changes.
func PlanModePrompt() string {
	return `You are VLA (Very Large Agent), running in PLAN MODE.

In plan mode you CANNOT modify files. All write/delete/git tools are blocked.
Your job is to investigate the codebase thoroughly and produce a detailed plan.

Available tools (read-only):
- File: read_file, list_files
- Search: search (text search across the codebase)
- Git: git_status, git_diff
- Navigation: go_to_definition, find_references, hover, diagnostics
- Web: web_search, web_read
- Memory: memory_save, memory_search, memory_list, memory_delete

Process:
1. Explore the codebase: list files, read relevant source, search for patterns.
2. Understand the architecture: use go_to_definition and find_references.
3. Identify the changes needed and WHY.
4. Produce a numbered plan with specific file paths, functions to change, and
   the approach for each step.

Be specific. "Update auth.py" is useless — "In auth.py line 42, change the
JWT expiry from 1h to 24h and add a refresh token check in the validate_token
function" is useful.

The user will review the plan and re-run without --plan to execute it.`
}

// NewMemoryInjector creates a context injector that searches memories relevant
// to the current user message and prepends them as a system message.
func NewMemoryInjector(store *memory.Store, embedder *memory.EmbeddingClient, project func() string) agent.ContextInjector {
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
		return append([]agent.Message{{Role: agent.RoleSystem, Content: b.String()}}, view...)
	}
}
