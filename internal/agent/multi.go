// Multi-agent parallel execution for VLA. The Coordinator spawns multiple
// agent loops concurrently, each working on an independent sub-task. Results
// are collected and merged. This is useful when a complex task can be
// decomposed into independent parts (e.g. "fix the bug" + "write tests" +
// "update docs" can run in parallel).
//
// Each sub-agent gets its own message history but shares the tool registry.
// The Coordinator waits for all sub-agents to complete, then returns their
// results as a combined summary.
package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/abrandt/vla/internal/tools"
)

// SubTask is one parallel task for the Coordinator.
type SubTask struct {
	Name   string // human-readable task name (e.g. "investigate", "write tests")
	Prompt string // the instruction for this sub-agent
}

// SubResult is the output of one parallel sub-agent.
type SubResult struct {
	Task     SubTask
	Response string // the final assistant response
	Error    error
}

// Coordinator runs multiple sub-agents in parallel. Each sub-agent is a
// lightweight agent loop that shares the LLM client and tool registry but
// has its own isolated message history.
type Coordinator struct {
	client    Streamer
	registry  *tools.Registry
	compactor Compactor
	threshold int
}

// NewCoordinator creates a Coordinator that can spawn parallel sub-agents.
// It reuses the same LLM client and tool registry as the main agent loop.
func NewCoordinator(client Streamer, registry *tools.Registry, compactor Compactor, threshold int) *Coordinator {
	return &Coordinator{
		client:    client,
		registry:  registry,
		compactor: compactor,
		threshold: threshold,
	}
}

// Run executes all sub-tasks in parallel and returns their results.
// Each sub-agent runs in its own goroutine with an independent message list.
// The system prompt and the sub-task prompt are prepended to each agent's
// messages. Results are returned in the same order as the input tasks.
func (c *Coordinator) Run(tasks []SubTask, systemPrompt string) []SubResult {
	results := make([]SubResult, len(tasks))
	var wg sync.WaitGroup

	for i, task := range tasks {
		wg.Add(1)
		go func(idx int, t SubTask) {
			defer wg.Done()
			results[idx] = c.runSubAgent(t, systemPrompt)
		}(i, task)
	}

	wg.Wait()
	return results
}

// runSubAgent runs one sub-agent to completion. It sends the prompt, gets
// the response, executes any tool calls, and returns the final text.
func (c *Coordinator) runSubAgent(task SubTask, systemPrompt string) SubResult {
	msgs := []Message{
		{Role: RoleSystem, Content: systemPrompt},
		{Role: RoleUser, Content: task.Prompt},
	}

	// Run the agent loop locally (not via Loop.Run which reads from stdin).
	for iter := 0; iter < MaxTurns; iter++ {
		view, err := c.compactor(msgs, func([]Message) (string, error) {
			return "compacted", nil
		}, c.threshold)
		if err != nil {
			return SubResult{Task: task, Error: err}
		}

		resp, err := c.client.StreamTo(view, c.registry.Schemas(), io.Discard)
		if err != nil {
			return SubResult{Task: task, Error: err}
		}
		msgs = append(msgs, resp)

		if len(resp.ToolCalls) == 0 {
			return SubResult{Task: task, Response: resp.Content}
		}

		// Execute tool calls.
		for _, tc := range resp.ToolCalls {
			result := c.executeToolCall(tc)
			msgs = append(msgs, Message{
				Role:       RoleTool,
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}

	return SubResult{
		Task:     task,
		Response: fmt.Sprintf("[reached max %d iterations]", MaxTurns),
	}
}

// executeToolCall is a simplified version that doesn't use approval/hooks
// (sub-agents are autonomous — they don't prompt the user).
func (c *Coordinator) executeToolCall(tc ToolCall) string {
	tool, ok := c.registry.Get(tc.Function.Name)
	if !ok {
		return fmt.Sprintf("Error: unknown tool %q", tc.Function.Name)
	}
	result, err := tool.Execute(json.RawMessage(tc.Function.Arguments))
	if err != nil {
		return fmt.Sprintf("Error: %s: %v", tc.Function.Name, err)
	}
	return result
}

// FormatResults renders the combined results of all sub-agents as a
// human-readable summary.
func FormatResults(results []SubResult) string {
	var b strings.Builder
	for _, r := range results {
		fmt.Fprintf(&b, "── %s ──\n", r.Task.Name)
		if r.Error != nil {
			fmt.Fprintf(&b, "Error: %v\n\n", r.Error)
		} else {
			fmt.Fprintf(&b, "%s\n\n", r.Response)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
