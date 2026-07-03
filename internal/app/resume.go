package app

import (
	"fmt"
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
