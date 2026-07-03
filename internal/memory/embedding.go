package memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// EmbeddingClient fetches embeddings from an OpenAI-compatible API
// (/v1/embeddings). It reuses the same API key and base URL as the LLM
// client, so no additional configuration is needed.
type EmbeddingClient struct {
	apiKey  string
	baseURL string
	model   string
	http    *http.Client
}

// NewEmbeddingClient returns a client for the OpenAI embeddings endpoint.
// model defaults to "text-embedding-3-small" if empty.
func NewEmbeddingClient(apiKey, baseURL, model string) *EmbeddingClient {
	if model == "" {
		model = "text-embedding-3-small"
	}
	return &EmbeddingClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Embed returns the embedding vector for a single text.
func (c *EmbeddingClient) Embed(text string) ([]float32, error) {
	vecs, err := c.EmbedBatch([]string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("embedding: empty response")
	}
	return vecs[0], nil
}

// EmbedBatch returns embedding vectors for multiple texts in one API call.
func (c *EmbeddingClient) EmbedBatch(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	body, err := json.Marshal(map[string]any{
		"input":           texts,
		"model":           c.model,
		"encoding_format": "float",
	})
	if err != nil {
		return nil, fmt.Errorf("embedding: marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embedding: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("embedding: decode: %w", err)
	}
	vecs := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		vecs[i] = d.Embedding
	}
	return vecs, nil
}
