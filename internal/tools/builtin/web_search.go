package builtin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// WebSearch searches the web and returns results. Uses DuckDuckGo's HTML
// endpoint (no API key required) as the default backend — deterministic
// enough for tool-use: we parse result titles + URLs.
//
// The HTTP client is injectable via the struct field so tests can pass an
// httptest.Server. Production leaves it nil → a default client with a
// 15s timeout is used.
type WebSearch struct {
	Client *http.Client // optional; nil → default with 15s timeout
}

func (WebSearch) Name() string { return "web_search" }

func (WebSearch) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "The search query.",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Max results to return. Default 5.",
			},
		},
		"required": []string{"query"},
	}
}

// SearchResult is one hit from a web search.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

func (w WebSearch) Execute(args json.RawMessage) (string, error) {
	var in struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fmt.Sprintf("Error: parse arguments: %v", err), nil
	}
	if in.Query == "" {
		return "Error: query is required", nil
	}
	if in.MaxResults <= 0 {
		in.MaxResults = 5
	}

	client := w.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	results, err := ddgSearch(client, in.Query, in.MaxResults)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	if len(results) == 0 {
		return "no results found", nil
	}

	var b strings.Builder
	for i, r := range results {
		fmt.Fprintf(&b, "%d. %s\n   %s\n", i+1, r.Title, r.URL)
		if r.Snippet != "" {
			fmt.Fprintf(&b, "   %s\n", r.Snippet)
		}
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// ddgSearch queries DuckDuckGo's HTML endpoint and parses results.
func ddgSearch(client *http.Client, query string, max int) ([]SearchResult, error) {
	endpoint := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "VLA/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseDDGResults(string(body), max), nil
}

// parseDDGResults extracts result titles + URLs from DuckDuckGo's HTML.
// DDG wraps results in <a class="result__a" href="...">Title</a>. The href
// is a redirect link containing the actual URL as a parameter.
func parseDDGResults(html string, max int) []SearchResult {
	var results []SearchResult
	for len(html) > 0 {
		// Find the next result link.
		idx := strings.Index(html, `class="result__a"`)
		if idx < 0 {
			break
		}
		html = html[idx:]
		// Extract href.
		hrefStart := strings.Index(html, `href="`)
		if hrefStart < 0 {
			break
		}
		hrefStart += 6
		hrefEnd := strings.IndexByte(html[hrefStart:], '"')
		if hrefEnd < 0 {
			break
		}
		href := html[hrefStart : hrefStart+hrefEnd]
		html = html[hrefStart+hrefEnd:]
		// Extract title text between > and </a>.
		titleStart := strings.IndexByte(html, '>')
		if titleStart < 0 {
			break
		}
		titleStart++
		titleEnd := strings.Index(html[titleStart:], "</a>")
		if titleEnd < 0 {
			break
		}
		title := strings.TrimSpace(html[titleStart : titleStart+titleEnd])
		title = decodeHTMLEntities(title)

		// DDG wraps the real URL in a redirect like
		// //duckduckgo.com/l/?uddg=<encoded-url>&...
		actualURL := extractDDGURL(href)
		if actualURL == "" {
			actualURL = href
		}

		results = append(results, SearchResult{Title: title, URL: actualURL})
		if len(results) >= max {
			break
		}
	}
	return results
}

// extractDDGURL pulls the real URL out of DDG's redirect wrapper.
func extractDDGURL(href string) string {
	if u, err := url.Parse(href); err == nil {
		if q := u.Query().Get("uddg"); q != "" {
			return q
		}
	}
	return ""
}

func decodeHTMLEntities(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	return s
}
