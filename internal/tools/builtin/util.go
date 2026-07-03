package builtin

// truncate shortens s to at most n chars, appending "…" if truncated.
// Shared by the builtin tools (memory, file, etc.).
func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
