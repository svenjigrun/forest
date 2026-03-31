package store_test

import (
	"testing"
	"time"

	"github.com/svenjigrun/forest/internal/id"
	"github.com/svenjigrun/forest/internal/node"
	"github.com/svenjigrun/forest/internal/store"
)

func makeNode(ct node.ContentType, body string) *node.Node {
	return &node.Node{
		ID:          id.New(),
		ContentType: ct,
		CreatedAt:   time.Now().UTC(),
		Provenance:  node.Provenance{Source: node.SourceUser},
		Body:        body,
	}
}

func TestPutAndGet(t *testing.T) {
	fs, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}

	n := makeNode(node.ContentTypeMarkdown, "hello world")
	if err := fs.Put(n); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := fs.Get(n.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != n.ID {
		t.Errorf("ID: got %q want %q", got.ID, n.ID)
	}
	if got.Body != n.Body {
		t.Errorf("Body: got %q want %q", got.Body, n.Body)
	}
	if got.ContentType != n.ContentType {
		t.Errorf("ContentType: got %q want %q", got.ContentType, n.ContentType)
	}
}

func TestGetNotFound(t *testing.T) {
	fs, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	_, err = fs.Get("00000000-0000-7000-8000-000000000000")
	if err == nil {
		t.Fatal("expected error for missing node, got nil")
	}
}

func TestList(t *testing.T) {
	fs, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}

	a := makeNode(node.ContentTypeMarkdown, "alpha")
	b := makeNode(node.ContentTypeJSON, "beta")
	c := makeNode(node.ContentTypeSchema, "gamma")

	for _, n := range []*node.Node{a, b, c} {
		if err := fs.Put(n); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}

	nodes, err := fs.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(nodes) != 3 {
		t.Errorf("List count: got %d want 3", len(nodes))
	}
}

func TestSubdirectoryRouting(t *testing.T) {
	root := t.TempDir()
	fs, err := store.New(root)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}

	cases := []struct {
		ct      node.ContentType
		wantDir string
	}{
		{node.ContentTypeMarkdown, "nodes"},
		{node.ContentTypeJSON, "nodes"},
		{node.ContentTypePython, "nodes"},
		{node.ContentTypeSchema, "schemas"},
		{node.ContentTypeContext, "contexts"},
	}

	for _, tc := range cases {
		n := makeNode(tc.ct, "test")
		if err := fs.Put(n); err != nil {
			t.Fatalf("Put %s: %v", tc.ct, err)
		}
		path := fs.Path(n.ID, tc.ct)
		if path == "" {
			t.Errorf("Path returned empty for %s", tc.ct)
		}
	}
}
