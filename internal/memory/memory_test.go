package memory

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStore_SaveAndGet(t *testing.T) {
	s := NewStore(t.TempDir())
	m := &Memory{
		Project: "proj",
		Content: "The auth module uses JWT tokens",
		Tags:    []string{"auth", "jwt"},
	}
	if err := s.Save(m); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if m.ID == "" {
		t.Fatal("ID not set")
	}

	got, err := s.Get("proj", m.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Content != m.Content {
		t.Errorf("content = %q", got.Content)
	}
	if len(got.Tags) != 2 {
		t.Errorf("tags = %v", got.Tags)
	}
}

func TestStore_GetMissing(t *testing.T) {
	s := NewStore(t.TempDir())
	_, err := s.Get("proj", "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing memory")
	}
}

func TestStore_Delete(t *testing.T) {
	s := NewStore(t.TempDir())
	m := &Memory{Project: "proj", Content: "to be deleted"}
	_ = s.Save(m)
	if err := s.Delete("proj", m.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get("proj", m.ID); err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestStore_List(t *testing.T) {
	s := NewStore(t.TempDir())
	m1 := &Memory{Project: "proj", Content: "first"}
	m2 := &Memory{Project: "proj", Content: "second"}
	_ = s.Save(m1)
	_ = s.Save(m2)

	got, err := s.List("proj")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2, got %d", len(got))
	}
}

func TestStore_ListEmptyProject(t *testing.T) {
	s := NewStore(t.TempDir())
	got, err := s.List("nonexistent")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0, got %d", len(got))
	}
}

func TestStore_SaveValidation(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := s.Save(&Memory{Content: "no project"}); err == nil {
		t.Error("expected error for missing project")
	}
	if err := s.Save(&Memory{Project: "p"}); err == nil {
		t.Error("expected error for missing content")
	}
}

func TestSearch_KeywordMatch(t *testing.T) {
	s := NewStore(t.TempDir())
	_ = s.Save(&Memory{Project: "p", Content: "The database uses PostgreSQL", Tags: []string{"db"}})
	_ = s.Save(&Memory{Project: "p", Content: "Authentication via OAuth2"})

	results, err := s.Search("p", "database", nil, 10, 0.7, 0.3)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !contains(results[0].Memory.Content, "PostgreSQL") {
		t.Errorf("wrong result: %q", results[0].Memory.Content)
	}
}

func TestSearch_TagMatch(t *testing.T) {
	s := NewStore(t.TempDir())
	_ = s.Save(&Memory{Project: "p", Content: "config file", Tags: []string{"config"}})
	_ = s.Save(&Memory{Project: "p", Content: "unrelated stuff"})

	results, _ := s.Search("p", "config", nil, 10, 0.7, 0.3)
	if len(results) != 1 {
		t.Fatalf("expected 1 tag match, got %d", len(results))
	}
}

func TestSearch_NoMatch(t *testing.T) {
	s := NewStore(t.TempDir())
	_ = s.Save(&Memory{Project: "p", Content: "hello world"})

	results, _ := s.Search("p", "nonexistent_query_xyz", nil, 10, 0.7, 0.3)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearch_VectorFusion(t *testing.T) {
	s := NewStore(t.TempDir())
	// Two memories with known embeddings — both must have nonzero similarity
	// to the query vector to be considered hits.
	m1 := &Memory{Project: "p", Content: "close match", Embedding: []float32{1, 0, 0}}
	m2 := &Memory{Project: "p", Content: "far match", Embedding: []float32{0.5, 0.5, 0}}
	_ = s.Save(m1)
	_ = s.Save(m2)

	// Query vector close to m1 (sim≈1.0) and less close to m2 (sim≈0.707).
	// Query text matches neither → keyword=0 for both. Pure vector ranking.
	queryVec := []float32{0.9, 0.1, 0}
	results, err := s.Search("p", "zzznomatch", queryVec, 10, 1.0, 0.0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Memory.Content != "close match" {
		t.Errorf("expected close match first, got %q", results[0].Memory.Content)
	}
}

func TestSearch_Limit(t *testing.T) {
	s := NewStore(t.TempDir())
	for i := 0; i < 5; i++ {
		_ = s.Save(&Memory{Project: "p", Content: "common word here"})
	}
	results, _ := s.Search("p", "common", nil, 2, 0.7, 0.3)
	if len(results) != 2 {
		t.Errorf("expected 2 results (capped), got %d", len(results))
	}
}

func TestCosineSim(t *testing.T) {
	cases := []struct {
		a, b []float32
		want float64
	}{
		{[]float32{1, 0}, []float32{1, 0}, 1.0},
		{[]float32{1, 0}, []float32{0, 1}, 0.0},
		{[]float32{1, 0}, []float32{-1, 0}, -1.0},
		{[]float32{}, []float32{1}, 0.0},
		{[]float32{1, 1}, []float32{1, 1}, 1.0},
	}
	for _, c := range cases {
		got := cosineSim(c.a, c.b)
		if abs(got-c.want) > 1e-9 {
			t.Errorf("cosineSim(%v, %v) = %f, want %f", c.a, c.b, got, c.want)
		}
	}
}

func TestEmbeddingClient_HTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	}))
	defer srv.Close()

	c := NewEmbeddingClient("key", srv.URL, "test-model")
	vec, err := c.Embed("hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 3 || vec[0] != 0.1 {
		t.Errorf("vec = %v", vec)
	}
}

func TestEmbeddingClient_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer srv.Close()

	c := NewEmbeddingClient("key", srv.URL, "test-model")
	_, err := c.Embed("hello")
	if err == nil {
		t.Fatal("expected error for 401")
	}
}

func TestSanitizeProject(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"myproject", "myproject"},
		{"/home/user/proj", "proj"},
		{"", "default"},
		{".", "default"},
	}
	for _, c := range cases {
		got := sanitizeProject(c.in)
		if got != c.want {
			t.Errorf("sanitizeProject(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
