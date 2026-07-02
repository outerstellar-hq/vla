package builtin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// WebRead fetches a URL and returns its text content. HTML pages are
// stripped to text (tags removed, scripts/styles dropped). Plain text
// and JSON are returned as-is. Capped at MaxReadBytes to protect the
// context window.
type WebRead struct {
	Client *http.Client // optional; nil → default with 15s timeout
}

func (WebRead) Name() string { return "web_read" }

func (WebRead) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "The URL to fetch.",
			},
			"max_bytes": map[string]any{
				"type":        "integer",
				"description": "Max bytes to return. Default 256 KiB.",
			},
		},
		"required": []string{"url"},
	}
}

func (w WebRead) Execute(args json.RawMessage) (string, error) {
	var in struct {
		URL      string `json:"url"`
		MaxBytes int    `json:"max_bytes"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fmt.Sprintf("Error: parse arguments: %v", err), nil
	}
	if in.URL == "" {
		return "Error: url is required", nil
	}
	if !strings.HasPrefix(in.URL, "http://") && !strings.HasPrefix(in.URL, "https://") {
		return "Error: url must start with http:// or https://", nil
	}
	if in.MaxBytes <= 0 || in.MaxBytes > 512*1024 {
		in.MaxBytes = 256 * 1024
	}

	client := w.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	req, err := http.NewRequest(http.MethodGet, in.URL, nil)
	if err != nil {
		return fmt.Sprintf("Error: build request: %v", err), nil
	}
	req.Header.Set("User-Agent", "VLA/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Sprintf("Error: fetch %s: %v", in.URL, err), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Sprintf("Error: HTTP %d for %s", resp.StatusCode, in.URL), nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(in.MaxBytes)))
	if err != nil {
		return fmt.Sprintf("Error: read body: %v", err), nil
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") {
		return htmlToText(string(body)), nil
	}
	return string(body), nil
}

// htmlToText strips HTML tags, scripts, styles, and decodes entities to
// produce readable text. It's deliberately crude — good enough for the LLM
// to extract information from a page without a full DOM parser.
func htmlToText(html string) string {
	// Drop script and style blocks entirely.
	html = stripBlock(html, "<script", "</script>")
	html = stripBlock(html, "<style", "</style>")
	html = stripBlock(html, "<!--", "-->")

	// Remove all remaining tags.
	var b strings.Builder
	inTag := false
	for _, r := range html {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	text := b.String()
	text = decodeHTMLEntities(text)
	// Collapse runs of whitespace into single newlines/spaces.
	text = collapseWhitespace(text)
	return strings.TrimSpace(text)
}

// stripBlock removes everything from startTag to endTag (case-insensitive).
func stripBlock(s, startTag, endTag string) string {
	lower := strings.ToLower(s)
	var b strings.Builder
	idx := 0
	for {
		start := strings.Index(lower[idx:], strings.ToLower(startTag))
		if start < 0 {
			b.WriteString(s[idx:])
			break
		}
		b.WriteString(s[idx : idx+start])
		end := strings.Index(lower[idx+start:], strings.ToLower(endTag))
		if end < 0 {
			break // unterminated block; drop the rest
		}
		idx = idx + start + end + len(endTag)
	}
	return b.String()
}

// collapseWhitespace turns runs of whitespace into single spaces, but
// preserves newlines.
func collapseWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		fields := strings.Fields(line)
		lines[i] = strings.Join(fields, " ")
	}
	return strings.Join(lines, "\n")
}
