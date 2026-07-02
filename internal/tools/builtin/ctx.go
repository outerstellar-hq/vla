// Package builtin holds VLA's built-in tools. Each tool is a self-contained
// struct in its own file implementing tools.Tool.
package builtin

// Ctx carries the shared state every filesystem/git tool needs: the project
// root (BaseDir) that path arguments are confined to, and limits.
// Tools receive it via a struct field set at registration time in main.go.
type Ctx struct {
	BaseDir string // absolute path to the project root; all paths confined here
}
