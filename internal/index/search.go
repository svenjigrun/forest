package index

import (
	"fmt"
	"time"

	"github.com/svenjigrun/forest/internal/node"
)

// Search returns up to limit nodes matching query using BM25 full-text search.
// An empty query returns all nodes ordered by created_at descending.
func (idx *Index) Search(query string, limit int) ([]*node.Node, error) {
	if query == "" {
		return idx.listRecent(limit)
	}

	// Ensure the FTS index exists. PRAGMA create_fts_index is idempotent.
	_, err := idx.db.Exec(`PRAGMA create_fts_index('nodes', 'id', 'content', overwrite=1)`)
	if err != nil {
		return nil, fmt.Errorf("create fts index: %w", err)
	}

	rows, err := idx.db.Query(`
		SELECT n.id, n.content_type, n.content, n.created_at, n.recency_score
		FROM nodes n
		JOIN (
			SELECT id, fts_main_nodes.match_bm25(id, ?) AS score
			FROM nodes
		) s ON s.id = n.id
		WHERE s.score IS NOT NULL
		ORDER BY s.score DESC
		LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()
	return scanNodes(rows)
}

func (idx *Index) listRecent(limit int) ([]*node.Node, error) {
	rows, err := idx.db.Query(
		`SELECT id, content_type, content, created_at, recency_score FROM nodes ORDER BY created_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNodes(rows)
}

func scanNodes(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]*node.Node, error) {
	var nodes []*node.Node
	for rows.Next() {
		var (
			n            node.Node
			createdAt    time.Time
			recencyScore float64
		)
		if err := rows.Scan(&n.ID, &n.ContentType, &n.Body, &createdAt, &recencyScore); err != nil {
			return nil, err
		}
		n.CreatedAt = createdAt
		nodes = append(nodes, &n)
	}
	return nodes, rows.Err()
}
