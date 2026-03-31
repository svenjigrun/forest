package index_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/svenjigrun/forest/internal/id"
	"github.com/svenjigrun/forest/internal/index"
	"github.com/svenjigrun/forest/internal/node"
	"github.com/svenjigrun/forest/internal/store"
)

func TestWatcher_Create(t *testing.T) {
	root := t.TempDir()
	seedFixtures(t, root)

	idx, err := index.Build(root)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer idx.Close()

	w, err := index.NewWatcher(root, idx)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Stop()

	// Write a new node file directly.
	fs, _ := store.New(root)
	n := &node.Node{
		ID:          id.New(),
		ContentType: node.ContentTypeMarkdown,
		CreatedAt:   time.Now().UTC(),
		Provenance:  node.Provenance{Source: node.SourceUser},
		Body:        "Watcher test node.",
	}
	if err := fs.Put(n); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Wait for the watcher to pick up the change.
	select {
	case ev := <-w.Events():
		if ev.ID != n.ID {
			t.Errorf("event ID: got %q want %q", ev.ID, n.ID)
		}
		if ev.Op != index.OpUpsert {
			t.Errorf("event Op: got %v want Upsert", ev.Op)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watcher create event")
	}

	count, err := idx.CountNodes()
	if err != nil {
		t.Fatalf("CountNodes: %v", err)
	}
	if count != 6 { // 5 fixtures + 1 new
		t.Errorf("node count after create: got %d want 6", count)
	}
}

func TestWatcher_Modify(t *testing.T) {
	root := t.TempDir()
	fixtures := seedFixtures(t, root)

	idx, err := index.Build(root)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer idx.Close()

	w, err := index.NewWatcher(root, idx)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Stop()

	// Overwrite the first fixture file.
	target := fixtures[0]
	target.Body = "Updated body content."
	fs, _ := store.New(root)
	if err := fs.Put(target); err != nil {
		t.Fatalf("Put (modify): %v", err)
	}

	select {
	case ev := <-w.Events():
		if ev.Op != index.OpUpsert {
			t.Errorf("modify event Op: got %v want Upsert", ev.Op)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watcher modify event")
	}
}

func TestWatcher_Delete(t *testing.T) {
	root := t.TempDir()
	fixtures := seedFixtures(t, root)

	idx, err := index.Build(root)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer idx.Close()

	w, err := index.NewWatcher(root, idx)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Stop()

	// Delete the first fixture file.
	target := fixtures[0]
	fs, _ := store.New(root)
	path := fs.Path(target.ID, target.ContentType)
	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	select {
	case ev := <-w.Events():
		if ev.Op != index.OpDelete {
			t.Errorf("delete event Op: got %v want Delete", ev.Op)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watcher delete event")
	}

	count, err := idx.CountNodes()
	if err != nil {
		t.Fatalf("CountNodes: %v", err)
	}
	if count != 4 {
		t.Errorf("node count after delete: got %d want 4", count)
	}
}

// seedFixtures is in index_test.go; this file reuses it via the same package.
// We add a helper here to get the path for deletion.
func nodePath(root string, n *node.Node) string {
	return filepath.Join(root, "nodes", n.ID+".md")
}
