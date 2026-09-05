package app

import (
	"fmt"
	"os"
)

func ResolveProjectPath(args []string) (string, error) {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("cody: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("cody: %s is not a directory", path)
	}
	return path, nil
}
