package index

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/rjeczalik/notify"
)

// Op describes what happened to a node in the file tree.
type Op int

const (
	OpUpsert Op = iota
	OpDelete
)

// Event is emitted by Watcher when a node file changes.
type Event struct {
	ID string
	Op Op
}

// Watcher watches the node directories and keeps the Index up to date.
type Watcher struct {
	root   string
	idx    *Index
	ch     chan notify.EventInfo
	events chan Event
	done   chan struct{}
}

// NewWatcher starts watching root and updating idx. Call Stop to release resources.
func NewWatcher(root string, idx *Index) (*Watcher, error) {
	ch := make(chan notify.EventInfo, 64)
	events := make(chan Event, 64)

	// Watch all subdirectories recursively.
	if err := notify.Watch(filepath.Join(root, "..."), ch,
		notify.Create, notify.Write, notify.Remove, notify.Rename,
	); err != nil {
		return nil, err
	}

	w := &Watcher{
		root:   root,
		idx:    idx,
		ch:     ch,
		events: events,
		done:   make(chan struct{}),
	}
	go w.run()
	return w, nil
}

// Events returns the channel on which file-change events are delivered.
func (w *Watcher) Events() <-chan Event {
	return w.events
}

// Stop stops the watcher and closes the events channel.
func (w *Watcher) Stop() {
	notify.Stop(w.ch)
	close(w.done)
}

func (w *Watcher) run() {
	for {
		select {
		case <-w.done:
			return
		case ei := <-w.ch:
			path := ei.Path()
			if !strings.HasSuffix(path, ".md") {
				continue
			}
			nodeID := strings.TrimSuffix(filepath.Base(path), ".md")

			switch ei.Event() {
			case notify.Remove, notify.Rename:
				_ = w.idx.DeleteNode(nodeID)
				w.emit(Event{ID: nodeID, Op: OpDelete})
			case notify.Create, notify.Write:
				f, err := os.Open(path)
				if err != nil {
					continue
				}
				n, err := parseNodeFile(f)
				f.Close()
				if err != nil {
					continue
				}
				_ = w.idx.UpsertNode(n)
				w.emit(Event{ID: n.ID, Op: OpUpsert})
			}
		}
	}
}

func (w *Watcher) emit(ev Event) {
	select {
	case w.events <- ev:
	default:
		// Drop if consumer is not reading; index is already updated.
	}
}
