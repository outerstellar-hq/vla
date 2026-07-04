// init_cmd.go — the `vla init` subcommand: sets up .vla/ directory,
// config, and example files for a new project.
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func runInitCmd(args []string) {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vla init: %v\n", err)
		os.Exit(1)
	}

	vlaDir := filepath.Join(absDir, ".vla")
	fmt.Printf("Initializing VLA project in %s\n", absDir)

	// Create .vla/ directory.
	if err := os.MkdirAll(vlaDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "vla init: cannot create %s: %v\n", vlaDir, err)
		os.Exit(1)
	}
	fmt.Printf("  ✓ Created %s/\n", filepath.Join(".vla"))

	// Create config.json if it doesn't exist.
	configPath := filepath.Join(absDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configContent := `{
  "api_key": "",
  "base_url": "https://api.openai.com/v1",
  "model": "gpt-4o"
}
`
		if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "vla init: cannot write config.json: %v\n", err)
		} else {
			fmt.Printf("  ✓ Created config.json (edit with your API key)\n")
		}
	} else {
		fmt.Printf("  • config.json already exists (skipped)\n")
	}

	// Create example files (only if they don't exist).
	examples := map[string]string{
		".vla/permissions.json.example": `{
  "default": "ask",
  "rules": {
    "write_file": "ask",
    "update_file": "ask",
    "delete_file": "deny",
    "git_commit": "ask",
    "read_file": "allow",
    "search": "allow",
    "list_files": "allow"
  }
}
`,
		".vla/hooks.json.example": `{
  "hooks": [
    {
      "event": "on_write",
      "tool": "",
      "command": "gofmt -w $VLA_FILE 2>/dev/null || true"
    }
  ]
}
`,
		".vla/steering.md": `# Project Steering Message

This file is prepended to the system prompt for every session in this project.
Use it to define project conventions, coding standards, architecture decisions,
or any persistent instructions the agent should follow.

Example:
- We use tabs, not spaces.
- All new code must have unit tests.
- Never modify the migration files directly.
`,
	}

	for relPath, content := range examples {
		fullPath := filepath.Join(absDir, relPath)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			os.MkdirAll(filepath.Dir(fullPath), 0o755)
			if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "vla init: cannot write %s: %v\n", relPath, err)
			} else {
				fmt.Printf("  ✓ Created %s\n", relPath)
			}
		} else {
			fmt.Printf("  • %s already exists (skipped)\n", relPath)
		}
	}

	// Create .vla/plugins/ directory.
	pluginsDir := filepath.Join(vlaDir, "plugins")
	os.MkdirAll(pluginsDir, 0o755)
	fmt.Printf("  ✓ Created %s/\n", filepath.Join(".vla", "plugins"))

	fmt.Printf("\nDone! Next steps:\n")
	fmt.Printf("  1. Edit config.json with your API key\n")
	fmt.Printf("  2. Or run: vla use openai/gpt-4o\n")
	fmt.Printf("  3. Run:   vla\n")
	fmt.Printf("\nDocumentation: https://github.com/outerstellar-hq/vla/tree/main/docs\n")
}
