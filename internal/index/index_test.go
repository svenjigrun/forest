package index_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/svenjigrun/forest/internal/id"
	"github.com/svenjigrun/forest/internal/index"
	"github.com/svenjigrun/forest/internal/node"
	"github.com/svenjigrun/forest/internal/store"
)

func seedFixtures(t *testing.T, root string) []*node.Node {
	t.Helper()
	fs, err := store.New(root)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	nodes := []*node.Node{
		{
			ID: id.New(), ContentType: node.ContentTypeMarkdown,
			CreatedAt: time.Now().UTC(), Provenance: node.Provenance{Source: node.SourceUser},
			Body: "Alpha node about the graph store.",
		},
		{
			ID: id.New(), ContentType: node.ContentTypeMarkdown,
			CreatedAt: time.Now().UTC(), Provenance: node.Provenance{Source: node.SourceUser},
			Body: "Beta node about context inference.",
		},
		{
			ID: id.New(), ContentType: node.ContentTypeMarkdown,
			CreatedAt: time.Now().UTC(), Provenance: node.Provenance{Source: node.SourceUser},
			Body: "Gamma node about embedding models.",
		},
	}
	// Add a node with a link to the first.
	linked := &node.Node{
		ID: id.New(), ContentType: node.ContentTypeMarkdown,
		CreatedAt: time.Now().UTC(), Provenance: node.Provenance{Source: node.SourceUser},
		Links:    []node.Link{{Type: node.LinkCites, TargetID: nodes[0].ID}},
		Contexts: []string{"project:forest"},
		Body:     "Delta node that cites Alpha.",
	}
	// Schema node.
	schema := &node.Node{
		ID: id.New(), ContentType: node.ContentTypeSchema,
		CreatedAt: time.Now().UTC(), Provenance: node.Provenance{Source: node.SourceUser},
		Body: "Contact schema.",
	}
	all := append(nodes, linked, schema)
	for _, n := range all {
		if err := fs.Put(n); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}
	return all
}

func TestBuild_RowCounts(t *testing.T) {
	root := t.TempDir()
	fixtures := seedFixtures(t, root)

	idx, err := index.Build(root)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer idx.Close()

	nodeCount, err := idx.CountNodes()
	if err != nil {
		t.Fatalf("CountNodes: %v", err)
	}
	if nodeCount != len(fixtures) {
		t.Errorf("node count: got %d want %d", nodeCount, len(fixtures))
	}

	edgeCount, err := idx.CountEdges()
	if err != nil {
		t.Fatalf("CountEdges: %v", err)
	}
	if edgeCount != 1 {
		t.Errorf("edge count: got %d want 1", edgeCount)
	}
}

func TestBuild_Idempotent(t *testing.T) {
	root := t.TempDir()
	seedFixtures(t, root)

	idx, err := index.Build(root)
	if err != nil {
		t.Fatalf("first Build: %v", err)
	}
	idx.Close()

	idx2, err := index.Build(root)
	if err != nil {
		t.Fatalf("second Build: %v", err)
	}
	defer idx2.Close()

	count, err := idx2.CountNodes()
	if err != nil {
		t.Fatalf("CountNodes after second build: %v", err)
	}
	if count != 5 {
		t.Errorf("count after rebuild: got %d want 5", count)
	}
}

func TestBuild_DBPath(t *testing.T) {
	root := t.TempDir()
	seedFixtures(t, root)

	idx, err := index.Build(root)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer idx.Close()

	expected := filepath.Join(root, ".forest", "index.duckdb")
	if idx.Path() != expected {
		t.Errorf("DB path: got %q want %q", idx.Path(), expected)
	}
}
