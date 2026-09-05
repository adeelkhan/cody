package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProjectPathDefaultsToCwd(t *testing.T) {
	path, err := ResolveProjectPath(nil)
	if err != nil {
		t.Fatal(err)
	}
	if path != "." {
		t.Fatalf("got %q, want \".\"", path)
	}
}

func TestResolveProjectPathValidDir(t *testing.T) {
	dir := t.TempDir()
	path, err := ResolveProjectPath([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if path != dir {
		t.Fatalf("got %q, want %q", path, dir)
	}
}

func TestResolveProjectPathMissing(t *testing.T) {
	_, err := ResolveProjectPath([]string{"/does/not/exist"})
	if err == nil {
		t.Fatal("expected an error for a missing path")
	}
}

func TestResolveProjectPathNotADir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveProjectPath([]string{file})
	if err == nil {
		t.Fatal("expected an error for a non-directory path")
	}
}
