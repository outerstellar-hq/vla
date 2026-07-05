package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// MaxConsecutiveErrors is the number of consecutive tool errors allowed
// before the agent locks and returns control to the user. Prevents the LLM
// from retrying bad tool calls in a tight loop.
const MaxConsecutiveErrors = 3

// errorLogEntry is one logged tool error, written as NDJSON.
type errorLogEntry struct {
	Timestamp string `json:"timestamp"`
	Session   string `json:"session"`
	Tool      string `json:"tool"`
	Args      string `json:"args"`
	Error     string `json:"error"`
}

// logDir returns the directory for VLA log files.
func logDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".vla"
	}
	return filepath.Join(home, ".vla", "logs")
}

// LogToolError appends a tool error to the error log file as NDJSON.
// Best-effort: errors in logging don't crash the loop.
func LogToolError(sessionID, toolName, args, errMsg string) {
	dir := logDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	entry := errorLogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Session:   sessionID,
		Tool:      toolName,
		Args:      args,
		Error:     errMsg,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	path := filepath.Join(dir, "tool-errors.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	f.Write(data)
	f.Write([]byte("\n"))
}
