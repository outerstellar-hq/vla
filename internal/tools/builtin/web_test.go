package builtin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebRead_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "hello world")
	}))
	defer srv.Close()

	w := WebRead{Client: srv.Client()}
	out, err := w.Execute(json.RawMessage(`{"url":"` + srv.URL + `"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "hello world" {
		t.Errorf("got %q", out)
	}
}

func TestWebRead_HTMLStrips(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head><title>Ignored</title><script>var x=1;</script><style>body{}</style></head><body><h1>Title</h1><p>Content here</p></body></html>`)
	}))
	defer srv.Close()

	w := WebRead{Client: srv.Client()}
	out, _ := w.Execute(json.RawMessage(`{"url":"` + srv.URL + `"}`))
	if strings.Contains(out, "<") {
		t.Errorf("HTML tags not stripped: %q", out)
	}
	if strings.Contains(out, "var x") {
		t.Errorf("script content not removed: %q", out)
	}
	if strings.Contains(out, "body{}") {
		t.Errorf("style content not removed: %q", out)
	}
	if !strings.Contains(out, "Title") || !strings.Contains(out, "Content here") {
		t.Errorf("expected text content, got %q", out)
	}
}

func TestWebRead_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	w := WebRead{Client: srv.Client()}
	out, _ := w.Execute(json.RawMessage(`{"url":"` + srv.URL + `"}`))
	if !strings.Contains(out, "Error:") || !strings.Contains(out, "404") {
		t.Errorf("expected 404 error, got %q", out)
	}
}

func TestWebRead_MissingURL(t *testing.T) {
	w := WebRead{}
	out, _ := w.Execute(json.RawMessage(`{}`))
	if !strings.Contains(out, "url is required") {
		t.Errorf("got %q", out)
	}
}

func TestWebRead_NonHTTPRejected(t *testing.T) {
	w := WebRead{}
	out, _ := w.Execute(json.RawMessage(`{"url":"ftp://example.com"}`))
	if !strings.Contains(out, "must start with http") {
		t.Errorf("got %q", out)
	}
}

func TestWebRead_EntityDecode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<p>Tom &amp; Jerry &lt;3</p>")
	}))
	defer srv.Close()

	w := WebRead{Client: srv.Client()}
	out, _ := w.Execute(json.RawMessage(`{"url":"` + srv.URL + `"}`))
	if !strings.Contains(out, "Tom & Jerry") {
		t.Errorf("entities not decoded: %q", out)
	}
	if !strings.Contains(out, "<3") {
		t.Errorf("lt entity not decoded: %q", out)
	}
}

// TestWebSearch_ParsesResults uses a fake DDG HTML response to verify the
// parser extracts titles and URLs correctly — no real network.
func TestWebSearch_ParsesResults(t *testing.T) {
	fakeHTML := `
<div class="results">
  <a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fpage1&rut=xyz">First Result</a>
  <a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.org%2Fpage2&rut=abc">Second &amp; Result</a>
</div>`
	results := parseDDGResults(fakeHTML, 5)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Title != "First Result" {
		t.Errorf("result 0 title = %q", results[0].Title)
	}
	if results[0].URL != "https://example.com/page1" {
		t.Errorf("result 0 url = %q", results[0].URL)
	}
	if results[1].Title != "Second & Result" {
		t.Errorf("result 1 title (entity decode) = %q", results[1].Title)
	}
	if results[1].URL != "https://example.org/page2" {
		t.Errorf("result 1 url = %q", results[1].URL)
	}
}

func TestWebSearch_MaxResultsCap(t *testing.T) {
	fakeHTML := ""
	for i := 0; i < 10; i++ {
		fakeHTML += fmt.Sprintf(`<a class="result__a" href="//duckduckgo.com/l/?uddg=https%%3A%%2F%%2Fexample.com%%2F%d">Result %d</a>`, i, i)
	}
	results := parseDDGResults(fakeHTML, 3)
	if len(results) != 3 {
		t.Errorf("expected 3 results (capped), got %d", len(results))
	}
}

func TestWebSearch_EmptyQuery(t *testing.T) {
	w := WebSearch{Client: &http.Client{}}
	out, _ := w.Execute(json.RawMessage(`{"query":""}`))
	if !strings.Contains(out, "query is required") {
		t.Errorf("got %q", out)
	}
}

func TestWebSearch_NoResults(t *testing.T) {
	// Empty HTML → no results parsed.
	results := parseDDGResults("<html><body>nothing here</body></html>", 5)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}
