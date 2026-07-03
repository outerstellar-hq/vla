// Package modelsdev also provides the CLI subcommands for browsing the
// model catalog and switching providers. These are separate from the
// agent loop — they're configuration utilities.
package modelsdev

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// PrintProviders lists all providers with model counts. If filter is non-empty,
// only providers whose name/ID contains the filter (case-insensitive) are shown.
func PrintProviders(providers map[string]Provider, filter string) {
	filter = strings.ToLower(filter)
	type entry struct {
		id    string
		name  string
		count int
		api   string
	}
	var entries []entry
	for id, p := range providers {
		if filter != "" && !strings.Contains(strings.ToLower(id), filter) &&
			!strings.Contains(strings.ToLower(p.Name), filter) {
			continue
		}
		entries = append(entries, entry{id: id, name: p.Name, count: len(p.Models), api: p.API})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].id < entries[j].id })

	if len(entries) == 0 {
		fmt.Println("no providers found")
		return
	}
	fmt.Printf("%d providers:\n", len(entries))
	for _, e := range entries {
		fmt.Printf("  %-20s %-20s %3d models  %s\n", e.id, e.name, e.count, e.api)
	}
}

// PrintModels lists models for a provider. If filter is non-empty, only
// models whose name/ID contains it are shown.
func PrintModels(providerID string, providers map[string]Provider, filter string) {
	p, ok := FindProvider(providers, providerID)
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown provider %q\n", providerID)
		os.Exit(1)
	}
	filter = strings.ToLower(filter)

	type entry struct {
		id      string
		name    string
		context int
		tools   bool
		cost    string
	}
	var entries []entry
	for _, m := range p.Models {
		if filter != "" && !strings.Contains(strings.ToLower(m.ID), filter) &&
			!strings.Contains(strings.ToLower(m.Name), filter) {
			continue
		}
		cost := "free"
		if m.Cost.Input > 0 {
			cost = fmt.Sprintf("$%.2f/$%.2f", m.Cost.Input, m.Cost.Output)
		}
		entries = append(entries, entry{
			id: m.ID, name: m.Name, context: m.Limit.Context,
			tools: m.ToolCall, cost: cost,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].id < entries[j].id })

	if len(entries) == 0 {
		fmt.Printf("no models found for %s matching %q\n", p.Name, filter)
		return
	}
	fmt.Printf("%s — %d models:\n", p.Name, len(entries))
	for _, e := range entries {
		toolFlag := ""
		if !e.tools {
			toolFlag = " [no tools]"
		}
		fmt.Printf("  %-35s %-25s ctx:%-8d %s%s\n", e.id, e.name, e.context, e.cost, toolFlag)
	}
}

// Select resolves a "provider/model" spec (like "openai/gpt-4o") into the
// full provider + model config. Returns the API key (from env), base URL,
// and model ID — everything needed to configure the LLM client.
type Selection struct {
	Provider Provider
	Model    Model
	APIKey   string // resolved from env, may be empty
	BaseURL  string
	ModelID  string
}

// Select parses "provider/model" and resolves it against the catalog.
func Select(providers map[string]Provider, spec string) (Selection, error) {
	parts := strings.SplitN(spec, "/", 2)
	if len(parts) != 2 {
		return Selection{}, fmt.Errorf("expected format: provider/model (e.g. openai/gpt-4o), got %q", spec)
	}
	providerID := parts[0]
	modelID := parts[1]
	// Handle nested model IDs like "xai/grok-4" where the provider is "requesty"
	// and the model is "xai/grok-4". Try provider/model first; if that fails,
	// check if providerID is actually a provider with modelID as a sub-prefix.
	p, m, found := FindModel(providers, providerID, modelID)
	if !found {
		// Maybe the model ID has a slash (e.g. requesty/xai/grok-4).
		// Try treating the whole thing as provider=providerID, model=modelID
		// with a more aggressive search.
		var providerFound bool
		p, providerFound = FindProvider(providers, providerID)
		if !providerFound {
			return Selection{}, fmt.Errorf("unknown provider %q — run 'vla models' to see available providers", providerID)
		}
		// Search all models for one whose ID contains modelID.
		var modelFound bool
		for mid, mdl := range p.Models {
			if strings.Contains(mid, modelID) {
				m = mdl
				modelFound = true
				break
			}
		}
		if !modelFound {
			return Selection{}, fmt.Errorf("no model matching %q in %s", modelID, p.Name)
		}
	}
	return Selection{
		Provider: p,
		Model:    m,
		APIKey:   ResolveAPIKey(p),
		BaseURL:  p.API,
		ModelID:  m.ID,
	}, nil
}
