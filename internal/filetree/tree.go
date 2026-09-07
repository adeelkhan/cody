package filetree

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type NodeType int

const (
	NodeFile NodeType = iota
	NodeDir
)

type Node struct {
	Name     string
	Path     string
	Type     NodeType
	Expanded bool
	Children []*Node
	loaded   bool
}

func NewRoot(rootPath string) (*Node, error) {
	info, err := os.Stat(rootPath)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &os.PathError{Op: "newroot", Path: rootPath, Err: os.ErrInvalid}
	}
	root := &Node{
		Name:     filepath.Base(rootPath),
		Path:     rootPath,
		Type:     NodeDir,
		Expanded: true,
	}
	if err := root.LoadChildren(); err != nil {
		return nil, err
	}
	return root, nil
}

func (n *Node) LoadChildren() error {
	if n.Type != NodeDir || n.loaded {
		return nil
	}
	return n.reloadChildren()
}

// Reload re-reads this directory's entries from disk unconditionally (even
// if already loaded), adding new entries and dropping deleted ones, while
// preserving the existing *Node — and its Expanded/loaded/Children state —
// for every entry still present. A no-op on a non-directory node.
func (n *Node) Reload() error {
	if n.Type != NodeDir {
		return nil
	}
	return n.reloadChildren()
}

func (n *Node) reloadChildren() error {
	entries, err := os.ReadDir(n.Path)
	if err != nil {
		return err
	}
	existing := make(map[string]*Node, len(n.Children))
	for _, c := range n.Children {
		existing[c.Name] = c
	}
	var children []*Node
	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		if c, ok := existing[e.Name()]; ok {
			children = append(children, c)
			continue
		}
		childType := NodeFile
		if e.IsDir() {
			childType = NodeDir
		}
		children = append(children, &Node{
			Name: e.Name(),
			Path: filepath.Join(n.Path, e.Name()),
			Type: childType,
		})
	}
	sort.Slice(children, func(i, j int) bool {
		if children[i].Type != children[j].Type {
			return children[i].Type == NodeDir
		}
		return children[i].Name < children[j].Name
	})
	n.Children = children
	n.loaded = true
	return nil
}

// CreateFile creates an empty file at path, failing if it already exists.
func CreateFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	return f.Close()
}

// CreateDir creates a directory at path, failing if it already exists.
func CreateDir(path string) error {
	return os.Mkdir(path, 0755)
}

// Rename renames oldPath to newPath, failing if newPath already exists.
func Rename(oldPath, newPath string) error {
	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("%s already exists", filepath.Base(newPath))
	}
	return os.Rename(oldPath, newPath)
}
