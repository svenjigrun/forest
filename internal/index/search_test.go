package index_test

import (
	"testing"
	"time"

	"github.com/svenjigrun/forest/internal/id"
	"github.com/svenjigrun/forest/internal/index"
	"github.com/svenjigrun/forest/internal/node"
	"github.com/svenjigrun/forest/internal/store"
)

func TestSearch_ReturnsMatchingNodes(t *testing.T) {
	root := t.TempDir()
	fs, err := store.New(root)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}

	nodes := []struct {
		body    string
		wantHit bool
	}{
		{"the graph store uses DuckDB for indexing", true},
		{"context inference relies on embedding similarity", false},
		{"DuckDB supports full-text search natively", true},
		{"the node lifecycle includes write enrich split merge", false},
		{"DuckDB is an in-process analytical database", true},
		{"ActivityPub is the federation protocol", false},
		{"ageing nodes recede from the active context", false},
		{"prompt nodes are executable with an LLM runtime", false},
		{"federated wiki uses side-by-side navigation", false},
		{"DuckDB also supports external Parquet files via httpfs", true},
	}

	var ids []string
	for _, tc := range nodes {
		n := &node.Node{
			ID:          id.New(),
			ContentType: node.ContentTypeMarkdown,
			CreatedAt:   time.Now().UTC(),
			Provenance:  node.Provenance{Source: node.SourceUser},
			Body:        tc.body,
		}
		if err := fs.Put(n); err != nil {
			t.Fatalf("Put: %v", err)
		}
		ids = append(ids, n.ID)
	}

	idx, err := index.Build(root)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer idx.Close()

	results, err := idx.Search("DuckDB", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(results) != 4 {
		t.Errorf("Search returned %d results, want 4", len(results))
	}

	// All returned nodes must contain "DuckDB" in their body.
	for _, r := range results {
		found := false
		for _, tc := range nodes {
			if tc.body == r.Body && tc.wantHit {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("unexpected result body: %q", r.Body)
		}
	}
}

func TestSearch_EmptyQuery_ReturnsAll(t *testing.T) {
	root := t.TempDir()
	seedFixtures(t, root)

	idx, err := index.Build(root)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer idx.Close()

	results, err := idx.Search("", 10)
	if err != nil {
		t.Fatalf("Search empty: %v", err)
	}
	if len(results) == 0 {
		t.Error("empty query returned no results")
	}
}

func TestSearch_Limit(t *testing.T) {
	root := t.TempDir()
	fs, _ := store.New(root)
	for range 20 {
		n := &node.Node{
			ID:          id.New(),
			ContentType: node.ContentTypeMarkdown,
			CreatedAt:   time.Now().UTC(),
			Provenance:  node.Provenance{Source: node.SourceUser},
			Body:        "forest node content",
		}
		_ = fs.Put(n)
	}

	idx, err := index.Build(root)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer idx.Close()

	results, err := idx.Search("forest", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) > 5 {
		t.Errorf("Search returned %d results, want ≤5", len(results))
	}
}
