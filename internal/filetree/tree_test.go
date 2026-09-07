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

func TestCreateFileMakesAnEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.go")
	if err := CreateFile(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("got %d bytes, want 0 (empty file)", len(data))
	}
}

func TestCreateFileFailsIfAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.go")
	mustWriteFile(t, path, "content")
	if err := CreateFile(path); err == nil {
		t.Fatal("expected an error creating a file that already exists")
	}
}

func TestCreateDirMakesADirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "newdir")
	if err := CreateDir(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal("expected a directory")
	}
}

func TestCreateDirFailsIfAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existingdir")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err := CreateDir(path); err == nil {
		t.Fatal("expected an error creating a directory that already exists")
	}
}

func TestRenameMovesTheFile(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.go")
	newPath := filepath.Join(dir, "new.go")
	mustWriteFile(t, oldPath, "content")
	if err := Rename(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatal("expected the old path to no longer exist")
	}
	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "content" {
		t.Fatalf("got %q, want %q", string(data), "content")
	}
}

func TestRenameFailsIfTargetAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "a.go")
	newPath := filepath.Join(dir, "b.go")
	mustWriteFile(t, oldPath, "a")
	mustWriteFile(t, newPath, "b")
	if err := Rename(oldPath, newPath); err == nil {
		t.Fatal("expected an error renaming onto an existing file")
	}
	data, _ := os.ReadFile(oldPath)
	if string(data) != "a" {
		t.Fatal("expected the source file to be untouched after a failed rename")
	}
}

func TestReloadPicksUpNewFilesWithoutLosingSiblingExpandedState(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(sub, "inner.go"), "")
	root, err := NewRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Expand "sub" so it has loaded children and Expanded == true.
	var subNode *Node
	for _, c := range root.Children {
		if c.Name == "sub" {
			subNode = c
		}
	}
	if subNode == nil {
		t.Fatal("setup failed: expected a 'sub' child")
	}
	subNode.Expanded = true
	if err := subNode.LoadChildren(); err != nil {
		t.Fatal(err)
	}

	// Create a new sibling file at the root, then reload the root.
	mustWriteFile(t, filepath.Join(dir, "new.go"), "")
	if err := root.Reload(); err != nil {
		t.Fatal(err)
	}

	found := false
	for _, c := range root.Children {
		if c.Name == "new.go" {
			found = true
		}
		if c.Name == "sub" && !c.Expanded {
			t.Fatal("expected 'sub' to remain expanded after reloading its unrelated sibling's parent")
		}
	}
	if !found {
		t.Fatal("expected the newly created file to appear after Reload")
	}
}
