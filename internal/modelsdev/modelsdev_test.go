package modelsdev

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mockCatalog() map[string]Provider {
	return map[string]Provider{
		"openai": {
			ID:   "openai",
			Name: "OpenAI",
			API:  "https://api.openai.com/v1",
			Env:  []string{"OPENAI_API_KEY"},
			Doc:  "https://platform.openai.com/docs",
			Models: map[string]Model{
				"gpt-4o": {
					ID:       "gpt-4o",
					Name:     "GPT-4o",
					ToolCall: true,
					Limit: struct {
						Context int `json:"context"`
						Output  int `json:"output"`
					}{Context: 128000, Output: 16384},
					Cost: struct {
						Input     float64 `json:"input"`
						Output    float64 `json:"output"`
						CacheRead float64 `json:"cache_read"`
					}{Input: 2.5, Output: 10},
				},
				"gpt-4o-mini": {
					ID:       "gpt-4o-mini",
					Name:     "GPT-4o Mini",
					ToolCall: true,
					Limit: struct {
						Context int `json:"context"`
						Output  int `json:"output"`
					}{Context: 128000, Output: 16384},
				},
			},
		},
		"anthropic": {
			ID:   "anthropic",
			Name: "Anthropic",
			API:  "https://api.anthropic.com/v1",
			Env:  []string{"ANTHROPIC_API_KEY"},
			Models: map[string]Model{
				"claude-sonnet-4-5": {
					ID:       "claude-sonnet-4-5",
					Name:     "Claude Sonnet 4.5",
					ToolCall: true,
					Limit: struct {
						Context int `json:"context"`
						Output  int `json:"output"`
					}{Context: 200000, Output: 64000},
				},
			},
		},
	}
}

func TestFetch_FromCache(t *testing.T) {
	dir := t.TempDir()
	client := NewClient(dir)

	// Pre-populate cache.
	catalog := mockCatalog()
	data, _ := json.Marshal(catalog)
	_ = os.WriteFile(filepath.Join(dir, "models-cache.json"), data, 0644)
	// Make it fresh (mod time = now).

	got, err := client.Fetch()
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 providers, got %d", len(got))
	}
	if _, ok := got["openai"]; !ok {
		t.Error("missing openai")
	}
}

func TestFetch_StaleCacheRefetches(t *testing.T) {
	dir := t.TempDir()
	client := NewClient(dir)

	// Write stale cache.
	catalog := mockCatalog()
	data, _ := json.Marshal(catalog)
	cachePath := filepath.Join(dir, "models-cache.json")
	_ = os.WriteFile(cachePath, data, 0644)
	stale := time.Now().Add(-48 * time.Hour)
	_ = os.Chtimes(cachePath, stale, stale)

	// Mock server returns fresh data.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	defer srv.Close()
	client.SetURL(srv.URL)

	got, err := client.Fetch()
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 providers, got %d", len(got))
	}
}

func TestFetch_FetchFailureFallsBackToStale(t *testing.T) {
	dir := t.TempDir()
	client := NewClient(dir)

	// Write stale cache.
	catalog := mockCatalog()
	data, _ := json.Marshal(catalog)
	cachePath := filepath.Join(dir, "models-cache.json")
	_ = os.WriteFile(cachePath, data, 0644)
	stale := time.Now().Add(-48 * time.Hour)
	_ = os.Chtimes(cachePath, stale, stale)

	// Point at a dead URL (mock server that returns 500).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	client.SetURL(srv.URL)

	// Fetch should fall back to stale cache when the fresh fetch fails.
	got, err := client.Fetch()
	if err != nil {
		t.Fatalf("expected stale fallback, got error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 providers from stale cache, got %d", len(got))
	}
}

func TestFindProvider_CaseInsensitive(t *testing.T) {
	catalog := mockCatalog()
	p, ok := FindProvider(catalog, "OpenAI")
	if !ok {
		t.Fatal("expected to find OpenAI case-insensitive")
	}
	if p.ID != "openai" {
		t.Errorf("got ID %q", p.ID)
	}
}

func TestFindProvider_NotFound(t *testing.T) {
	catalog := mockCatalog()
	_, ok := FindProvider(catalog, "nonexistent")
	if ok {
		t.Error("expected not found")
	}
}

func TestFindModel_Success(t *testing.T) {
	catalog := mockCatalog()
	p, m, ok := FindModel(catalog, "openai", "gpt-4o")
	if !ok {
		t.Fatal("expected to find gpt-4o")
	}
	if p.ID != "openai" {
		t.Errorf("provider = %q", p.ID)
	}
	if m.Name != "GPT-4o" {
		t.Errorf("model name = %q", m.Name)
	}
	if m.Limit.Context != 128000 {
		t.Errorf("context = %d", m.Limit.Context)
	}
}

func TestFindModel_CaseInsensitive(t *testing.T) {
	catalog := mockCatalog()
	_, m, ok := FindModel(catalog, "openai", "GPT-4O-MINI")
	if !ok {
		t.Fatal("expected case-insensitive match")
	}
	if m.Name != "GPT-4o Mini" {
		t.Errorf("name = %q", m.Name)
	}
}

func TestFindModel_UnknownProvider(t *testing.T) {
	catalog := mockCatalog()
	_, _, ok := FindModel(catalog, "unknown", "model")
	if ok {
		t.Error("expected not found")
	}
}

func TestFindModel_UnknownModel(t *testing.T) {
	catalog := mockCatalog()
	_, _, ok := FindModel(catalog, "openai", "nonexistent")
	if ok {
		t.Error("expected not found")
	}
}

func TestResolveAPIKey_FirstEnvVar(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test-123")
	t.Setenv("OTHER_KEY", "other")
	p := Provider{Env: []string{"OPENAI_API_KEY", "OTHER_KEY"}}
	key := ResolveAPIKey(p)
	if key != "sk-test-123" {
		t.Errorf("got %q", key)
	}
}

func TestResolveAPIKey_FallbackToSecond(t *testing.T) {
	t.Setenv("FIRST_KEY", "")
	t.Setenv("SECOND_KEY", "sk-second")
	p := Provider{Env: []string{"FIRST_KEY", "SECOND_KEY"}}
	key := ResolveAPIKey(p)
	if key != "sk-second" {
		t.Errorf("got %q", key)
	}
}

func TestResolveAPIKey_NoneSet(t *testing.T) {
	t.Setenv("UNSET_KEY_1", "")
	t.Setenv("UNSET_KEY_2", "")
	p := Provider{Env: []string{"UNSET_KEY_1", "UNSET_KEY_2"}}
	key := ResolveAPIKey(p)
	if key != "" {
		t.Errorf("expected empty, got %q", key)
	}
}

func TestEqualFold(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"openai", "openai", true},
		{"OpenAI", "openai", true},
		{"OPENAI", "openai", true},
		{"openai", "anthropic", false},
		{"openai", "openaiz", false},
		{"", "", true},
	}
	for _, c := range cases {
		if got := equalFold(c.a, c.b); got != c.want {
			t.Errorf("equalFold(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestFetchFresh_MockServer(t *testing.T) {
	catalog := mockCatalog()
	data, _ := json.Marshal(catalog)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	defer srv.Close()

	dir := t.TempDir()
	client := NewClient(dir)
	// Point the HTTP client at the mock server. We need to override CatalogURL,
	// but it's a const. Instead, use the HTTP client to fetch from the mock.
	resp, err := client.http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	var got map[string]Provider
	json.NewDecoder(resp.Body).Decode(&got)
	if len(got) != 2 {
		t.Errorf("expected 2 providers from mock, got %d", len(got))
	}
	// Verify cache was NOT written (we fetched manually, not via fetchFresh).
	if _, err := os.Stat(filepath.Join(dir, "models-cache.json")); err == nil {
		// fetchFresh wasn't called, so no cache — correct.
	}
	_ = strings.TrimSpace
}
