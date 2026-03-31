package node_test

import (
	"strings"
	"testing"
	"time"

	"github.com/svenjigrun/forest/internal/node"
)

func TestRoundTripProse(t *testing.T) {
	n := &node.Node{
		ID:          "01234567-89ab-7def-8123-456789abcdef",
		ContentType: node.ContentTypeMarkdown,
		CreatedAt:   time.Date(2026, 3, 31, 9, 0, 0, 0, time.UTC),
		Provenance:  node.Provenance{Source: node.SourceUser},
		Links: []node.Link{
			{Type: node.LinkCites, TargetID: "aaaaaaaa-bbbb-7ccc-8ddd-eeeeeeeeeeee"},
		},
		Contexts: []string{"project:forest"},
		Body:     "The paragraph is the atom of meaning.",
	}

	var buf strings.Builder
	if err := n.Encode(&buf); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	got, err := node.ParseFrom(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("ParseFrom: %v", err)
	}

	if got.ID != n.ID {
		t.Errorf("ID: got %q want %q", got.ID, n.ID)
	}
	if got.ContentType != n.ContentType {
		t.Errorf("ContentType: got %q want %q", got.ContentType, n.ContentType)
	}
	if !got.CreatedAt.Equal(n.CreatedAt) {
		t.Errorf("CreatedAt: got %v want %v", got.CreatedAt, n.CreatedAt)
	}
	if got.Provenance.Source != n.Provenance.Source {
		t.Errorf("Provenance.Source: got %q want %q", got.Provenance.Source, n.Provenance.Source)
	}
	if len(got.Links) != 1 || got.Links[0].Type != node.LinkCites || got.Links[0].TargetID != n.Links[0].TargetID {
		t.Errorf("Links: got %+v want %+v", got.Links, n.Links)
	}
	if len(got.Contexts) != 1 || got.Contexts[0] != "project:forest" {
		t.Errorf("Contexts: got %v want %v", got.Contexts, n.Contexts)
	}
	if got.Body != n.Body {
		t.Errorf("Body: got %q want %q", got.Body, n.Body)
	}
}

func TestRoundTripExecutable(t *testing.T) {
	n := &node.Node{
		ID:          "01234567-89ab-7def-8123-456789abcde0",
		ContentType: node.ContentTypePython,
		CreatedAt:   time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
		Provenance:  node.Provenance{Source: node.SourceUser},
		Runtime:     "python",
		Trust:       node.TrustUser,
		Inputs:      []string{"aaaaaaaa-bbbb-7ccc-8ddd-eeeeeeeeeeee"},
		Body:        "print('hello')",
	}

	var buf strings.Builder
	if err := n.Encode(&buf); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	got, err := node.ParseFrom(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("ParseFrom: %v", err)
	}

	if got.ContentType != node.ContentTypePython {
		t.Errorf("ContentType: got %q want %q", got.ContentType, node.ContentTypePython)
	}
	if got.Runtime != "python" {
		t.Errorf("Runtime: got %q want %q", got.Runtime, "python")
	}
	if got.Trust != node.TrustUser {
		t.Errorf("Trust: got %q want %q", got.Trust, node.TrustUser)
	}
	if len(got.Inputs) != 1 || got.Inputs[0] != n.Inputs[0] {
		t.Errorf("Inputs: got %v want %v", got.Inputs, n.Inputs)
	}
	if got.Body != n.Body {
		t.Errorf("Body: got %q want %q", got.Body, n.Body)
	}
}

func TestRoundTripSchemaNode(t *testing.T) {
	n := &node.Node{
		ID:          "01234567-89ab-7def-8123-456789abcde1",
		ContentType: node.ContentTypeSchema,
		CreatedAt:   time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
		Provenance:  node.Provenance{Source: node.SourceUser},
		Body:        "Contact schema v1.",
	}

	var buf strings.Builder
	if err := n.Encode(&buf); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	got, err := node.ParseFrom(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("ParseFrom: %v", err)
	}

	if got.ContentType != node.ContentTypeSchema {
		t.Errorf("ContentType: got %q want %q", got.ContentType, node.ContentTypeSchema)
	}
}

func TestRoundTripContextNode(t *testing.T) {
	n := &node.Node{
		ID:          "01234567-89ab-7def-8123-456789abcde2",
		ContentType: node.ContentTypeContext,
		CreatedAt:   time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		Provenance:  node.Provenance{Source: node.SourceUser},
		Body:        "project:forest context.",
	}

	var buf strings.Builder
	if err := n.Encode(&buf); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	got, err := node.ParseFrom(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("ParseFrom: %v", err)
	}

	if got.ContentType != node.ContentTypeContext {
		t.Errorf("ContentType: got %q want %q", got.ContentType, node.ContentTypeContext)
	}
}

func TestRoundTripStructured(t *testing.T) {
	n := &node.Node{
		ID:          "01234567-89ab-7def-8123-456789abcde3",
		ContentType: node.ContentTypeJSON,
		CreatedAt:   time.Date(2026, 4, 1, 13, 0, 0, 0, time.UTC),
		Provenance:  node.Provenance{Source: node.SourceUser},
		Body:        `{"name":"Alice","active":true}`,
	}

	var buf strings.Builder
	if err := n.Encode(&buf); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	got, err := node.ParseFrom(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("ParseFrom: %v", err)
	}

	if got.Body != n.Body {
		t.Errorf("Body: got %q want %q", got.Body, n.Body)
	}
}
