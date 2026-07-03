// Package memory implements VLA's persistent memory system, inspired by
// Memwizard. Memories are stored as JSON files per project under
// ~/.vla/memory/<project>/. Each memory carries content, tags, a timestamp,
// and an embedding vector for semantic search.
//
// Search is hybrid: keyword (substring match on content + tags) fused with
// vector cosine similarity, min-max normalized and weighted (0.7 vector /
// 0.3 keyword by default). This matches the Memwizard algorithm.
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Memory is one stored memory unit. It maps 1:1 to a JSON file on disk.
type Memory struct {
	ID        string    `json:"id"`
	Project   string    `json:"project"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Embedding []float32 `json:"embedding,omitempty"` // may be nil if embeddings disabled
}

// Store manages memory persistence for a project. Memories live as individual
// JSON files under root/<project>/<id>.json. File-based storage keeps the
// project dependency-free (no embedded database) and makes memories
// human-inspectable.
type Store struct {
	root string // ~/.vla/memory
}

// NewStore creates a Store rooted at the given directory (typically
// filepath.Join(home, ".vla", "memory")).
func NewStore(root string) *Store {
	return &Store{root: root}
}

// DefaultRoot returns ~/.vla/memory.
func DefaultRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".vla", "memory")
}

// projectDir returns the directory for a project's memories.
func (s *Store) projectDir(project string) string {
	return filepath.Join(s.root, sanitizeProject(project))
}

// sanitizeProject strips path separators from a project name so it's a safe
// single directory component.
func sanitizeProject(project string) string {
	project = filepath.Base(project)
	project = strings.ReplaceAll(project, string(filepath.Separator), "_")
	if project == "" || project == "." || project == "/" {
		project = "default"
	}
	return project
}

// Save writes a memory to disk. If m.ID is empty, a new timestamp-based ID
// is generated. Returns the memory as stored (with ID + timestamp filled in).
func (s *Store) Save(m *Memory) error {
	if m.Project == "" {
		return fmt.Errorf("memory: project is required")
	}
	if m.Content == "" {
		return fmt.Errorf("memory: content is required")
	}
	if m.ID == "" {
		m.ID = generateID()
	}
	if m.Timestamp.IsZero() {
		m.Timestamp = time.Now()
	}

	dir := s.projectDir(m.Project)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("memory: create project dir: %w", err)
	}
	path := filepath.Join(dir, m.ID+".json")
	data, err := jsonMarshal(m)
	if err != nil {
		return fmt.Errorf("memory: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("memory: write: %w", err)
	}
	return nil
}

// Get retrieves a memory by ID within a project.
func (s *Store) Get(project, id string) (*Memory, error) {
	path := filepath.Join(s.projectDir(project), id+".json")
	return s.load(path)
}

// Delete removes a memory by ID within a project.
func (s *Store) Delete(project, id string) error {
	path := filepath.Join(s.projectDir(project), id+".json")
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("memory: delete: %w", err)
	}
	return nil
}

// List returns all memories for a project, sorted by timestamp descending.
// If project is empty, returns an error.
func (s *Store) List(project string) ([]*Memory, error) {
	dir := s.projectDir(project)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("memory: list: %w", err)
	}
	var memories []*Memory
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		m, err := s.load(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue // skip corrupt files
		}
		memories = append(memories, m)
	}
	sortByTimestampDesc(memories)
	return memories, nil
}

// load reads one memory file.
func (s *Store) load(path string) (*Memory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Memory
	if err := jsonUnmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// loadAll loads every memory across all projects (used by search when no
// project filter is given, or for dedup).
func (s *Store) loadAll() ([]*Memory, error) {
	var memories []*Memory
	projects, err := os.ReadDir(s.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, projDir := range projects {
		if !projDir.IsDir() {
			continue
		}
		ms, _ := s.List(projDir.Name())
		memories = append(memories, ms...)
	}
	return memories, nil
}

// Search searches memories by keyword (substring on content + tags) and,
// if a query embedding is provided, vector cosine similarity. Results are
// fused with min-max normalization: score = vectorWeight*vScore +
// keywordWeight*kScore. Only memories with a positive keyword OR vector
// score are returned.
func (s *Store) Search(project string, query string, queryVec []float32, limit int, vectorWeight, keywordWeight float64) ([]*SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	var candidates []*Memory
	var err error
	if project != "" {
		candidates, err = s.List(project)
	} else {
		candidates, err = s.loadAll()
	}
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	qLower := strings.ToLower(query)
	var hits []scored
	for _, m := range candidates {
		kScore := keywordScore(m, qLower)
		vScore := 0.0
		if len(queryVec) > 0 && len(m.Embedding) > 0 {
			vScore = cosineSim(queryVec, m.Embedding)
		}
		if kScore > 0 || vScore > 0 {
			hits = append(hits, scored{mem: m, vScore: vScore, kScore: kScore})
		}
	}
	if len(hits) == 0 {
		return nil, nil
	}

	// Min-max normalize each score independently.
	normalizeScores(hits, func(h scored) float64 { return h.vScore }, func(h scored, v float64) scored { h.vScore = v; return h })
	normalizeScores(hits, func(h scored) float64 { return h.kScore }, func(h scored, v float64) scored { h.kScore = v; return h })

	// Fuse and sort.
	results := make([]*SearchResult, 0, len(hits))
	for _, h := range hits {
		fused := vectorWeight*h.vScore + keywordWeight*h.kScore
		results = append(results, &SearchResult{
			Memory: h.mem,
			Score:  fused,
		})
	}
	sortResultsByScoreDesc(results)
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// SearchResult wraps a memory with its relevance score.
type SearchResult struct {
	Memory *Memory
	Score  float64
}

// keywordScore returns 0 if no keyword match, or a simple relevance score
// (1.0 if content contains the query, 0.5 if only tags match).
func keywordScore(m *Memory, qLower string) float64 {
	if qLower == "" {
		return 0
	}
	if strings.Contains(strings.ToLower(m.Content), qLower) {
		return 1.0
	}
	for _, tag := range m.Tags {
		if strings.Contains(strings.ToLower(tag), qLower) {
			return 0.5
		}
	}
	return 0
}
