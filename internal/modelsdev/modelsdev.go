// Package modelsdev fetches and caches the models.dev catalog — an
// open-source database of 150+ LLM providers and their models (context
// limits, pricing, capabilities). This lets VLA auto-discover providers
// and models instead of requiring manual base_url configuration.
//
// The catalog is ~3MB and cached at ~/.vla/models-cache.json for 24 hours.
// On first use VLA fetches it once; subsequent launches use the cache.
package modelsdev

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// CatalogURL is the canonical models.dev API endpoint.
const CatalogURL = "https://models.dev/api.json"

// CacheTTL is how long the cached catalog is considered fresh.
const CacheTTL = 24 * time.Hour

// Provider describes one LLM provider in the catalog.
type Provider struct {
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	API    string           `json:"api"`    // base URL for OpenAI-compatible calls
	Env    []string         `json:"env"`    // env var names for the API key (e.g. ["OPENAI_API_KEY"])
	Doc    string           `json:"doc"`    // documentation URL
	NPM    string           `json:"npm"`    // AI SDK package (informational)
	Models map[string]Model `json:"models"` // keyed by model ID (e.g. "gpt-4o")
}

// Model describes one model's capabilities and limits.
type Model struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Family      string `json:"family"`
	Attachment  bool   `json:"attachment"`  // supports image/file input
	Reasoning   bool   `json:"reasoning"`   // reasoning model
	ToolCall    bool   `json:"tool_call"`   // supports function calling
	Temperature bool   `json:"temperature"` // supports temperature param
	Knowledge   string `json:"knowledge"`   // training data cutoff
	Limit       struct {
		Context int `json:"context"` // max context window in tokens
		Output  int `json:"output"`  // max output tokens
	} `json:"limit"`
	Cost struct {
		Input     float64 `json:"input"`      // $ per 1M input tokens
		Output    float64 `json:"output"`     // $ per 1M output tokens
		CacheRead float64 `json:"cache_read"` // $ per 1M cached input tokens
	} `json:"cost"`
}

// Client fetches and caches the models.dev catalog.
type Client struct {
	http     *http.Client
	cacheDir string // ~/.vla
	url      string // catalog URL (defaults to CatalogURL, overridable for tests)
}

// NewClient creates a Client that caches at the given directory.
func NewClient(cacheDir string) *Client {
	return &Client{
		http:     &http.Client{Timeout: 30 * time.Second},
		cacheDir: cacheDir,
		url:      CatalogURL,
	}
}

// SetURL overrides the catalog URL (for testing with a mock server).
func (c *Client) SetURL(url string) {
	c.url = url
}

// DefaultCacheDir returns ~/.vla.
func DefaultCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".vla")
}

// cachePath returns the path to the cached catalog file.
func (c *Client) cachePath() string {
	return filepath.Join(c.cacheDir, "models-cache.json")
}

// Fetch returns the catalog, using the cache if fresh, or fetching from
// models.dev if stale or missing. Returns a parsed map of provider ID → Provider.
func (c *Client) Fetch() (map[string]Provider, error) {
	// Try cache first.
	if data, err := os.ReadFile(c.cachePath()); err == nil {
		if fresh, _ := isCacheFresh(c.cachePath()); fresh {
			var providers map[string]Provider
			if err := json.Unmarshal(data, &providers); err == nil {
				return providers, nil
			}
		}
	}

	// Fetch fresh.
	providers, err := c.fetchFresh()
	if err != nil {
		// Fall back to stale cache if available (better than nothing).
		if data, readErr := os.ReadFile(c.cachePath()); readErr == nil {
			var stale map[string]Provider
			if json.Unmarshal(data, &stale) == nil {
				return stale, nil
			}
		}
		return nil, err
	}
	return providers, nil
}

// fetchFresh downloads the catalog from models.dev and writes it to cache.
func (c *Client) fetchFresh() (map[string]Provider, error) {
	resp, err := c.http.Get(c.url)
	if err != nil {
		return nil, fmt.Errorf("models.dev: fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models.dev: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("models.dev: read body: %w", err)
	}
	var providers map[string]Provider
	if err := json.Unmarshal(data, &providers); err != nil {
		return nil, fmt.Errorf("models.dev: parse: %w", err)
	}
	// Write cache.
	if err := os.MkdirAll(c.cacheDir, 0755); err != nil {
		return providers, nil // cache failure is non-fatal
	}
	_ = os.WriteFile(c.cachePath(), data, 0644)
	return providers, nil
}

// isCacheFresh returns true if the cache file was modified within CacheTTL.
func isCacheFresh(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return time.Since(info.ModTime()) < CacheTTL, nil
}

// FindProvider looks up a provider by ID (case-insensitive).
func FindProvider(providers map[string]Provider, id string) (Provider, bool) {
	for pid, p := range providers {
		if equalFold(pid, id) {
			return p, true
		}
	}
	return Provider{}, false
}

// FindModel looks up a model within a provider by ID (case-insensitive).
// Model IDs in the catalog use "provider/model" format (e.g. "openai/gpt-4o").
// This function also handles the "model" part alone within a known provider.
func FindModel(providers map[string]Provider, providerID, modelID string) (Provider, Model, bool) {
	p, ok := FindProvider(providers, providerID)
	if !ok {
		return Provider{}, Model{}, false
	}
	for mid, m := range p.Models {
		if equalFold(mid, modelID) {
			return p, m, true
		}
	}
	return Provider{}, Model{}, false
}

// ResolveAPIKey checks the provider's env vars for an API key. Returns the
// first one found. Provider entries in models.dev list env vars like
// ["OPENAI_API_KEY"] — VLA reads them in order.
func ResolveAPIKey(p Provider) string {
	for _, envVar := range p.Env {
		if key := os.Getenv(envVar); key != "" {
			return key
		}
	}
	return ""
}

// equalFold is a simple ASCII case-insensitive compare (avoids strings import
// in this function).
func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}
