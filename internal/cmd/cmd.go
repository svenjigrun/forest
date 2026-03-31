// Package cmd implements the core logic for Forest CLI commands.
// The kong wiring in cmd/forest/main.go delegates to these functions so
// they can be tested without invoking the full CLI.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/svenjigrun/forest/internal/index"
	"github.com/svenjigrun/forest/internal/store"
)

const defaultConfig = `# Forest configuration
models:
  default: local
  providers:
    local:
      provider: ollama
      base_url: http://localhost:11434/v1
`

// Init creates the Forest directory layout and default config at dir.
// It is idempotent.
func Init(dir string) error {
	// store.New creates nodes/, schemas/, contexts/, .forest/
	if _, err := store.New(dir); err != nil {
		return fmt.Errorf("init store: %w", err)
	}

	cfgPath := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		if err := os.WriteFile(cfgPath, []byte(defaultConfig), 0o644); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
	}
	return nil
}

// Reindex rebuilds the DuckDB index from the node files under root.
func Reindex(root string) error {
	idx, err := index.Build(root)
	if err != nil {
		return fmt.Errorf("build index: %w", err)
	}
	defer idx.Close()

	count, err := idx.CountNodes()
	if err != nil {
		return fmt.Errorf("count nodes: %w", err)
	}
	fmt.Printf("reindex: indexed %d nodes at %s\n", count, idx.Path())
	return nil
}
