package id_test

import (
	"sort"
	"testing"

	"github.com/svenjigrun/forest/internal/id"
)

func TestNew_Unique(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for range 1000 {
		v := id.New()
		if _, dup := seen[v]; dup {
			t.Fatalf("duplicate ID: %s", v)
		}
		seen[v] = struct{}{}
	}
}

func TestNew_MonotonicOrder(t *testing.T) {
	ids := make([]string, 100)
	for i := range ids {
		ids[i] = id.New()
	}
	sorted := make([]string, len(ids))
	copy(sorted, ids)
	sort.Strings(sorted)
	for i := range ids {
		if ids[i] != sorted[i] {
			t.Fatalf("IDs are not in lexicographic creation order at index %d: got %s want %s", i, sorted[i], ids[i])
		}
	}
}

func TestIsValid(t *testing.T) {
	if !id.IsValid(id.New()) {
		t.Fatal("IsValid returned false for a freshly generated ID")
	}
	if id.IsValid("not-a-uuid") {
		t.Fatal("IsValid returned true for an invalid string")
	}
	if id.IsValid("") {
		t.Fatal("IsValid returned true for an empty string")
	}
}
