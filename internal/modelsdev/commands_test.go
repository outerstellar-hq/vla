package modelsdev

import (
	"os"
	"strings"
	"testing"
)

func TestPrintProviders_All(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	PrintProviders(mockCatalog(), "")

	w.Close()
	os.Stdout = old
	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	out := string(buf[:n])

	if !strings.Contains(out, "openai") {
		t.Errorf("missing openai:\n%s", out)
	}
	if !strings.Contains(out, "anthropic") {
		t.Errorf("missing anthropic:\n%s", out)
	}
	if !strings.Contains(out, "2 providers") {
		t.Errorf("missing count:\n%s", out)
	}
}

func TestPrintProviders_Filtered(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	PrintProviders(mockCatalog(), "open")

	w.Close()
	os.Stdout = old
	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	out := string(buf[:n])

	if !strings.Contains(out, "openai") {
		t.Errorf("should contain openai:\n%s", out)
	}
	if strings.Contains(out, "anthropic") {
		t.Errorf("should NOT contain anthropic:\n%s", out)
	}
}

func TestPrintModels_ForProvider(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	PrintModels("openai", mockCatalog(), "")

	w.Close()
	os.Stdout = old
	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	out := string(buf[:n])

	if !strings.Contains(out, "GPT-4o") {
		t.Errorf("missing GPT-4o:\n%s", out)
	}
	if !strings.Contains(out, "GPT-4o Mini") {
		t.Errorf("missing mini:\n%s", out)
	}
	if !strings.Contains(out, "ctx:") {
		t.Errorf("missing context info:\n%s", out)
	}
}

func TestPrintModels_Filtered(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	PrintModels("openai", mockCatalog(), "mini")

	w.Close()
	os.Stdout = old
	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	out := string(buf[:n])

	if !strings.Contains(out, "Mini") {
		t.Errorf("should contain mini model:\n%s", out)
	}
	if strings.Contains(out, "GPT-4o\n") {
		t.Errorf("should NOT contain full GPT-4o:\n%s", out)
	}
}

func TestSelect_ValidSpec(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	sel, err := Select(mockCatalog(), "openai/gpt-4o")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if sel.Provider.ID != "openai" {
		t.Errorf("provider = %q", sel.Provider.ID)
	}
	if sel.Model.ID != "gpt-4o" {
		t.Errorf("model = %q", sel.Model.ID)
	}
	if sel.APIKey != "sk-test" {
		t.Errorf("api key = %q", sel.APIKey)
	}
	if sel.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("base url = %q", sel.BaseURL)
	}
}

func TestSelect_CaseInsensitive(t *testing.T) {
	sel, err := Select(mockCatalog(), "OpenAI/GPT-4O")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if sel.Model.ID != "gpt-4o" {
		t.Errorf("model = %q", sel.Model.ID)
	}
}

func TestSelect_MissingSlash(t *testing.T) {
	_, err := Select(mockCatalog(), "openai")
	if err == nil {
		t.Fatal("expected error for missing /")
	}
	if !strings.Contains(err.Error(), "provider/model") {
		t.Errorf("error = %v", err)
	}
}

func TestSelect_UnknownProvider(t *testing.T) {
	_, err := Select(mockCatalog(), "nonexistent/model")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown provider") {
		t.Errorf("error = %v", err)
	}
}

func TestSelect_UnknownModel(t *testing.T) {
	_, err := Select(mockCatalog(), "openai/nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSelect_NoAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	sel, err := Select(mockCatalog(), "openai/gpt-4o")
	if err != nil {
		t.Fatalf("Select should succeed even without key: %v", err)
	}
	if sel.APIKey != "" {
		t.Errorf("expected empty key, got %q", sel.APIKey)
	}
}
