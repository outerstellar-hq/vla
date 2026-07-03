package agent

// ContextInjector is called before each LLM request to optionally prepend
// relevant context (memories, project notes, etc.) to the message view.
// Returns nil to inject nothing. The agent loop calls this after compaction
// but before sending to the LLM.
//
// This is how the memory system auto-injects: the production injector queries
// the memory store with the latest user message as the query, then returns
// a system message summarizing the relevant memories.
type ContextInjector func(view []Message, lastUserMessage string) []Message
