// Package index manages the DuckDB-backed search index for Forest nodes.
//
// The index is a derived cache of the Markdown+YAML files on disk. It is
// stored at <root>/.forest/index.duckdb and can be fully rebuilt at any
// time by calling Build. Losing the file is never catastrophic.
package index

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/marcboeker/go-duckdb" // registers "duckdb" driver
	"github.com/svenjigrun/forest/internal/node"
	"github.com/svenjigrun/forest/internal/store"
)

const schema = `
CREATE TABLE IF NOT EXISTS nodes (
    id           TEXT PRIMARY KEY,
    content_type TEXT NOT NULL,
    content      TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    recency_score DOUBLE NOT NULL DEFAULT 1.0
);

CREATE TABLE IF NOT EXISTS edges (
    source_id  TEXT NOT NULL,
    target_id  TEXT NOT NULL,
    link_type  TEXT NOT NULL,
    PRIMARY KEY (source_id, target_id, link_type)
);

CREATE TABLE IF NOT EXISTS contexts (
    node_id     TEXT NOT NULL,
    context_tag TEXT NOT NULL,
    PRIMARY KEY (node_id, context_tag)
);
`

// Index is an open DuckDB-backed search index.
type Index struct {
	db   *sql.DB
	path string
}

// Build scans all node files under root, populates a fresh DuckDB index at
// <root>/.forest/index.duckdb, and returns the open Index.
// It is idempotent: calling Build twice produces the same result.
func Build(root string) (*Index, error) {
	dbPath := filepath.Join(root, ".forest", "index.duckdb")

	// Remove any existing file so the build is always clean.
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove old index: %w", err)
	}

	db, err := sql.Open("duckdb", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open duckdb: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	idx := &Index{db: db, path: dbPath}

	fs, err := store.New(root)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("open store: %w", err)
	}

	nodes, err := fs.List()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	for _, n := range nodes {
		if err := idx.upsertNode(n); err != nil {
			db.Close()
			return nil, fmt.Errorf("upsert node %s: %w", n.ID, err)
		}
	}

	return idx, nil
}

// Open opens an existing index at <root>/.forest/index.duckdb without rebuilding.
func Open(root string) (*Index, error) {
	dbPath := filepath.Join(root, ".forest", "index.duckdb")
	db, err := sql.Open("duckdb", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open duckdb: %w", err)
	}
	return &Index{db: db, path: dbPath}, nil
}

// Close releases the DuckDB connection.
func (idx *Index) Close() error {
	return idx.db.Close()
}

// Path returns the file path of the DuckDB database.
func (idx *Index) Path() string {
	return idx.path
}

// UpsertNode inserts or replaces a node and its edges in the index.
func (idx *Index) UpsertNode(n *node.Node) error {
	return idx.upsertNode(n)
}

// DeleteNode removes a node and its edges from the index.
func (idx *Index) DeleteNode(id string) error {
	tx, err := idx.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`DELETE FROM edges WHERE source_id = ? OR target_id = ?`, id, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM contexts WHERE node_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM nodes WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// CountNodes returns the total number of rows in the nodes table.
func (idx *Index) CountNodes() (int, error) {
	var n int
	err := idx.db.QueryRow(`SELECT COUNT(*) FROM nodes`).Scan(&n)
	return n, err
}

// CountEdges returns the total number of rows in the edges table.
func (idx *Index) CountEdges() (int, error) {
	var n int
	err := idx.db.QueryRow(`SELECT COUNT(*) FROM edges`).Scan(&n)
	return n, err
}

func (idx *Index) upsertNode(n *node.Node) error {
	tx, err := idx.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.Exec(
		`INSERT OR REPLACE INTO nodes (id, content_type, content, created_at) VALUES (?, ?, ?, ?)`,
		n.ID, string(n.ContentType), n.Body, n.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert node row: %w", err)
	}

	// Replace edges for this source node.
	if _, err := tx.Exec(`DELETE FROM edges WHERE source_id = ?`, n.ID); err != nil {
		return err
	}
	for _, l := range n.Links {
		if _, err := tx.Exec(
			`INSERT INTO edges (source_id, target_id, link_type) VALUES (?, ?, ?)`,
			n.ID, l.TargetID, string(l.Type),
		); err != nil {
			return fmt.Errorf("insert edge: %w", err)
		}
	}

	// Replace context memberships.
	if _, err := tx.Exec(`DELETE FROM contexts WHERE node_id = ?`, n.ID); err != nil {
		return err
	}
	for _, c := range n.Contexts {
		if _, err := tx.Exec(
			`INSERT INTO contexts (node_id, context_tag) VALUES (?, ?)`,
			n.ID, strings.TrimSpace(c),
		); err != nil {
			return fmt.Errorf("insert context: %w", err)
		}
	}

	return tx.Commit()
}
