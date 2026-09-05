package filetree

import (
	"os"
	"path/filepath"
	"testing"
)

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestNewRootSkipsGit(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, ".git"))
	mustMkdir(t, filepath.Join(dir, "src"))
	mustWriteFile(t, filepath.Join(dir, "main.go"), "package main")

	root, err := NewRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range root.Children {
		if c.Name == ".git" {
			t.Fatal(".git should be skipped")
		}
	}
	if len(root.Children) != 2 {
		t.Fatalf("got %d children, want 2", len(root.Children))
	}
}

func TestNewRootSortsDirsFirst(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "b.go"), "")
	mustMkdir(t, filepath.Join(dir, "a-dir"))

	root, err := NewRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if root.Children[0].Name != "a-dir" || root.Children[0].Type != NodeDir {
		t.Fatalf("expected dir first, got %+v", root.Children[0])
	}
}

func TestLoadChildrenLazy(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	mustMkdir(t, sub)
	mustWriteFile(t, filepath.Join(sub, "f.txt"), "")

	root, err := NewRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	subNode := root.Children[0]
	if subNode.Children != nil {
		t.Fatal("expected children not loaded until LoadChildren is called")
	}
	if err := subNode.LoadChildren(); err != nil {
		t.Fatal(err)
	}
	if len(subNode.Children) != 1 {
		t.Fatalf("got %d children, want 1", len(subNode.Children))
	}
}
