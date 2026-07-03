package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// IndexEntry is one session in the index.
type IndexEntry struct {
	ID         string    `json:"id"`
	Project    string    `json:"project"` // the CWD / project path
	Model      string    `json:"model"`
	Created    time.Time `json:"created"`
	LastActive time.Time `json:"last_active"`
}

// Index manages ~/.vla/sessions/index.json — a lookup table for sessions
// keyed by ID, with project path and timestamps. Enables cross-project
// session browsing and the `vla sessions` command.
type Index struct {
	path    string
	Entries map[string]IndexEntry `json:"entries"`
}

// LoadIndex reads (or creates) the session index.
func LoadIndex() *Index {
	indexPath := filepath.Join(SessionsDir(), "index.json")
	data, err := os.ReadFile(indexPath)
	idx := &Index{path: indexPath, Entries: make(map[string]IndexEntry)}
	if err == nil {
		json.Unmarshal(data, idx)
	}
	if idx.Entries == nil {
		idx.Entries = make(map[string]IndexEntry)
	}
	return idx
}

// Record adds or updates a session in the index.
func (idx *Index) Record(id, project, model string) {
	now := time.Now()
	entry := idx.Entries[id]
	entry.ID = id
	entry.Project = project
	entry.Model = model
	entry.LastActive = now
	if entry.Created.IsZero() {
		entry.Created = now
	}
	idx.Entries[id] = entry
	_ = idx.save()
}

// List returns all sessions sorted by last-active (most recent first).
func (idx *Index) List() []IndexEntry {
	list := make([]IndexEntry, 0, len(idx.Entries))
	for _, e := range idx.Entries {
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].LastActive.After(list[j].LastActive)
	})
	return list
}

// ListByProject returns sessions for a specific project path.
func (idx *Index) ListByProject(project string) []IndexEntry {
	var list []IndexEntry
	for _, e := range idx.Entries {
		if e.Project == project {
			list = append(list, e)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].LastActive.After(list[j].LastActive)
	})
	return list
}

// Remove deletes a session from the index.
func (idx *Index) Remove(id string) {
	delete(idx.Entries, id)
	_ = idx.save()
}

func (idx *Index) save() error {
	if idx.path == "" {
		return nil
	}
	_ = os.MkdirAll(filepath.Dir(idx.path), 0755)
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(idx.path, data, 0644)
}
