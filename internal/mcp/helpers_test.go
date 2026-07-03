package mcp

import (
	"os"
)

func osMkdirAll(path string) error {
	return os.MkdirAll(path, 0755)
}

func osWriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}
