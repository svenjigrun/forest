// Package store provides filesystem-backed persistence for Forest nodes.
//
// Nodes are stored as Markdown files with YAML frontmatter. The directory
// layout is:
//
//	<root>/nodes/       prose, JSON, executable, external-ref nodes
//	<root>/schemas/     schema nodes
//	<root>/contexts/    context nodes
//
// The DuckDB index is a derived cache of these files and is never the source
// of truth.
package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/svenjigrun/forest/internal/node"
)

// FileStore reads and writes nodes as Markdown+YAML files under a root directory.
type FileStore struct {
	root string
}

// New creates a FileStore rooted at dir, creating the required subdirectories.
func New(dir string) (*FileStore, error) {
	for _, sub := range []string{"nodes", "schemas", "contexts", ".forest"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return nil, fmt.Errorf("create %s: %w", sub, err)
		}
	}
	return &FileStore{root: dir}, nil
}

// Put writes n to disk, creating or overwriting the file.
func (fs *FileStore) Put(n *node.Node) error {
	path := fs.Path(n.ID, n.ContentType)
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()
	return n.Encode(f)
}

// Get reads the node with the given ID from disk.
func (fs *FileStore) Get(id string) (*node.Node, error) {
	// Search all subdirectories.
	for _, sub := range []string{"nodes", "schemas", "contexts"} {
		path := filepath.Join(fs.root, sub, id+".md")
		f, err := os.Open(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", path, err)
		}
		defer f.Close()
		return node.ParseFrom(f)
	}
	return nil, fmt.Errorf("node %q not found", id)
}

// List returns all nodes from all subdirectories.
func (fs *FileStore) List() ([]*node.Node, error) {
	var nodes []*node.Node
	for _, sub := range []string{"nodes", "schemas", "contexts"} {
		dir := filepath.Join(fs.root, sub)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			f, err := os.Open(path)
			if err != nil {
				return nil, fmt.Errorf("open %s: %w", path, err)
			}
			n, err := node.ParseFrom(f)
			f.Close()
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", path, err)
			}
			nodes = append(nodes, n)
		}
	}
	return nodes, nil
}

// Path returns the file path for a node with the given ID and content type.
func (fs *FileStore) Path(id string, ct node.ContentType) string {
	return filepath.Join(fs.root, subdir(ct), id+".md")
}

// subdir returns the subdirectory for a given content type.
func subdir(ct node.ContentType) string {
	switch ct {
	case node.ContentTypeSchema:
		return "schemas"
	case node.ContentTypeContext:
		return "contexts"
	default:
		return "nodes"
	}
}
