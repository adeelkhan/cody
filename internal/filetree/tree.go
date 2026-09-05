package filetree

import (
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
	entries, err := os.ReadDir(n.Path)
	if err != nil {
		return err
	}
	var children []*Node
	for _, e := range entries {
		if e.Name() == ".git" {
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
