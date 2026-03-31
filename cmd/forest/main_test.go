package main_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/svenjigrun/forest/internal/cmd"
)

func TestInit_CreatesLayout(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "myforest")

	if err := cmd.Init(target); err != nil {
		t.Fatalf("Init: %v", err)
	}

	for _, sub := range []string{"nodes", "schemas", "contexts", ".forest"} {
		if _, err := os.Stat(filepath.Join(target, sub)); err != nil {
			t.Errorf("missing directory %s: %v", sub, err)
		}
	}

	cfgPath := filepath.Join(target, "config.yaml")
	if _, err := os.Stat(cfgPath); err != nil {
		t.Errorf("missing config.yaml: %v", err)
	}
}

func TestInit_Idempotent(t *testing.T) {
	root := t.TempDir()
	if err := cmd.Init(root); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	if err := cmd.Init(root); err != nil {
		t.Fatalf("second Init: %v", err)
	}
}

func TestReindex_ProducesDuckDB(t *testing.T) {
	root := t.TempDir()
	if err := cmd.Init(root); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := cmd.Reindex(root); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	dbPath := filepath.Join(root, ".forest", "index.duckdb")
	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("DuckDB file not created: %v", err)
	}
}
