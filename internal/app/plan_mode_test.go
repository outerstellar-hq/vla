package app

import (
	"strings"
	"testing"
)

func TestPlanModePrompt(t *testing.T) {
	p := PlanModePrompt()
	if !strings.Contains(p, "PLAN MODE") {
		t.Error("plan prompt should mention PLAN MODE")
	}
	if !strings.Contains(p, "CANNOT modify") {
		t.Error("plan prompt should explain restrictions")
	}
	if !strings.Contains(p, "read_file") {
		t.Error("plan prompt should list read-only tools")
	}
	if strings.Contains(p, "write_file") && !strings.Contains(p, "blocked") {
		t.Error("plan prompt should not list write_file as available")
	}
}

func TestSystemPromptVsPlanMode(t *testing.T) {
	normal := SystemPrompt()
	plan := PlanModePrompt()
	if normal == plan {
		t.Error("normal and plan prompts should differ")
	}
}
