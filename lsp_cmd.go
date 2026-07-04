// lsp_cmd.go — the `vla lsp` subcommand: list, install, and check
// language servers. Each server has its own exact install spec — no
// generic fallbacks.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/abrandt/vla/internal/lsp"
)

func runLspCmd(args []string) {
	if len(args) == 0 {
		printLspHelp()
		return
	}

	switch args[0] {
	case "list":
		lspList()
	case "check":
		lspCheck()
	case "install":
		fs := flag.NewFlagSet("install", flag.ExitOnError)
		all := fs.Bool("all", false, "install all auto-installable servers")
		fs.Parse(args[1:])

		if *all {
			lspInstallAll()
		} else if fs.NArg() > 0 {
			lspInstallOne(fs.Arg(0))
		} else {
			fmt.Fprintln(os.Stderr, "usage: vla lsp install <language>  or  vla lsp install --all")
			os.Exit(1)
		}
	case "help", "-h", "--help":
		printLspHelp()
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n\n", args[0])
		printLspHelp()
		os.Exit(1)
	}
}

func printLspHelp() {
	fmt.Println(`vla lsp — manage language servers

Usage:
  vla lsp list              Show all servers with install status
  vla lsp check             Quick check: what's installed, what's missing
  vla lsp install <lang>    Install one language server
  vla lsp install --all     Install all auto-installable servers

Languages:
  python, go, kotlin, java, csharp, php, javascript,
  css, html, rust, ruby, c, dart, lua, elixir, scala, swift`)
}

func lspList() {
	specs := lsp.ListSpecs()
	fmt.Printf("%-12s  %-35s  %-8s  %-10s  %s\n",
		"LANGUAGE", "SERVER", "STATUS", "TOOL", "INSTALL COMMAND")
	fmt.Println(strings.Repeat("-", 100))

	installedCount := 0
	for _, s := range specs {
		status := "✓ installed"
		if !s.Installed {
			status = "✗ missing"
		} else {
			installedCount++
		}

		tool := string(s.InstallTool)
		cmd := s.FormatInstallCommand()

		fmt.Printf("%-12s  %-35s  %-8s  %-10s  %s\n",
			s.Language, s.Command, status, tool, cmd)
	}

	fmt.Println(strings.Repeat("-", 100))
	fmt.Printf("%d/%d installed\n", installedCount, len(specs))
}

func lspCheck() {
	specs := lsp.ListSpecs()
	installedCount := 0
	var missing []string

	for _, s := range specs {
		if s.Installed {
			installedCount++
		} else {
			missing = append(missing, string(s.Language))
		}
	}

	fmt.Printf("LSP servers: %d/%d installed\n", installedCount, len(specs))
	if len(missing) > 0 {
		fmt.Printf("Missing: %s\n", strings.Join(missing, ", "))
		fmt.Println("\nInstall missing servers:")
		fmt.Println("  vla lsp install --all")
	} else {
		fmt.Println("All language servers are installed.")
	}
}

func lspInstallOne(langName string) {
	lang := lsp.Language(strings.ToLower(langName))

	// Verify this is a known language.
	specs := lsp.DefaultSpecs()
	if _, ok := specs[lang]; !ok {
		fmt.Fprintf(os.Stderr, "vla: unknown language %q\n", langName)
		fmt.Fprintf(os.Stderr, "Supported: python, go, kotlin, java, csharp, php, javascript, css, html, rust, ruby, c, dart, lua, elixir, scala, swift\n")
		os.Exit(1)
	}

	installSpec := lsp.SpecForLanguage(lang)
	serverCmd := specs[lang].Command

	// Check if already installed.
	if _, err := exec.LookPath(serverCmd); err == nil {
		fmt.Printf("%s (%s): already installed\n", lang, serverCmd)
		return
	}

	// Manual installs can't be automated.
	if installSpec.Tool == lsp.InstallManual {
		fmt.Printf("%s (%s): cannot auto-install\n\n", lang, serverCmd)
		fmt.Println(installSpec.Instructions)
		return
	}

	// Check if the install tool is available.
	if !installSpec.IsAvailable() {
		fmt.Fprintf(os.Stderr, "vla: install tool not available for %s\n", lang)
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, installSpec.Instructions)
		os.Exit(1)
	}

	// Run the install.
	cmd := installSpec.CommandsForOS()
	fmt.Printf("Installing %s (%s)...\n", lang, serverCmd)
	fmt.Printf("  $ %s\n", strings.Join(cmd, " "))

	if err := installSpec.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "vla: install failed: %v\n", err)
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, installSpec.Instructions)
		os.Exit(1)
	}

	// Verify it's now on PATH.
	if _, err := exec.LookPath(serverCmd); err != nil {
		fmt.Fprintf(os.Stderr, "vla: install completed but %s not found on PATH\n", serverCmd)
		fmt.Fprintf(os.Stderr, "  You may need to restart your shell or add the install directory to PATH.\n")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, installSpec.Instructions)
		os.Exit(1)
	}

	fmt.Printf("✓ %s installed successfully\n", serverCmd)
}

func lspInstallAll() {
	specs := lsp.ListSpecs()
	installed := 0
	skipped := 0
	failed := 0

	for _, s := range specs {
		if s.Installed {
			continue
		}

		fmt.Printf("\n--- %s (%s) ---\n", s.Language, s.Command)
		lspInstallOne(string(s.Language))

		// Check if it worked.
		if _, err := exec.LookPath(s.Command); err == nil {
			installed++
		} else {
			failed++
		}
	}

	if installed == 0 && failed == 0 {
		fmt.Println("All auto-installable servers are already installed.")
	} else {
		fmt.Printf("\nDone: %d installed, %d failed, %d skipped\n", installed, failed, skipped)
	}
}
