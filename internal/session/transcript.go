package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// Append writes one turn as a single NDJSON line to the transcript file.
func (s *Session) Append(turn map[string]any) error {
	line, err := encodeLine(turn)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("session: open transcript for append: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("session: append turn: %w", err)
	}
	return nil
}

// Read returns the turns (excluding the metadata line) and the session metadata.
func (s *Session) Read() (turns []map[string]any, meta map[string]any, err error) {
	return readTranscript(s.path)
}

// readTranscript parses an NDJSON transcript file: line 1 is session metadata,
// every subsequent line is a turn.
func readTranscript(path string) ([]map[string]any, map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("session: open %s: %w", path, err)
	}
	defer f.Close()

	var meta map[string]any
	var turns []map[string]any
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err != nil {
			return nil, nil, fmt.Errorf("session: parse line %d of %s: %w", lineNo, path, err)
		}
		if t, _ := obj["type"].(string); t == "session" {
			meta = obj
			continue
		}
		turns = append(turns, obj)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("session: read %s: %w", path, err)
	}
	if meta == nil {
		return nil, nil, fmt.Errorf("session: %s has no session metadata line", path)
	}
	return turns, meta, nil
}

// encodeLine marshals obj to JSON with no trailing newline (Append adds it).
func encodeLine(obj map[string]any) ([]byte, error) {
	line, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("session: encode turn: %w", err)
	}
	return line, nil
}
