// Package node defines the core data types for Forest nodes.
//
// A node is the base unit of information: a chunk of content (prose, code,
// structured data, or a context/schema definition) with a stable ID, typed
// outbound links, provenance metadata, and a version history stored as git
// commits on the backing Markdown file.
package node

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/adrg/frontmatter"
)

// ContentType identifies what kind of content a node holds.
type ContentType string

const (
	ContentTypeMarkdown ContentType = "text/markdown"
	ContentTypeJSON     ContentType = "application/json"
	ContentTypePython   ContentType = "application/x-python"
	ContentTypeSQL      ContentType = "application/sql"
	ContentTypePrompt   ContentType = "application/x-prompt+llm"
	ContentTypeSchema   ContentType = "forest/schema"
	ContentTypeContext  ContentType = "forest/context"
	ContentTypeExtRef   ContentType = "forest/external-ref"
)

// LinkType describes the semantic relationship between two nodes.
type LinkType string

const (
	LinkCites          LinkType = "cites"
	LinkRespondsTo     LinkType = "responds-to"
	LinkContradicts    LinkType = "contradicts"
	LinkCorroborates   LinkType = "corroborates"
	LinkTranscludedFrom LinkType = "transcluded-from"
	LinkImportedFrom   LinkType = "imported-from"
	LinkSchema         LinkType = "schema"
)

// Source identifies where a node's content originated.
type Source string

const (
	SourceUser     Source = "user"
	SourceExternal Source = "external"
	SourceAI       Source = "ai"
)

// Trust describes the execution trust level of an executable node.
type Trust string

const (
	TrustUser      Trust = "user"
	TrustTrusted   Trust = "trusted"
	TrustUntrusted Trust = "untrusted"
)

// Link is a typed directed edge from this node to another.
type Link struct {
	Type      LinkType `yaml:"type"`
	TargetID  string   `yaml:"target"`
	TargetURL string   `yaml:"target_url,omitempty"`
}

// Provenance records where a node's content came from.
type Provenance struct {
	Source      Source `yaml:"source"`
	ImportedURL string `yaml:"imported_url,omitempty"`
}

// frontmatterFields mirrors the YAML block written at the top of a node file.
// It is unexported; callers use Node directly.
type frontmatterFields struct {
	ID          string      `yaml:"id"`
	ContentType ContentType `yaml:"content_type"`
	CreatedAt   time.Time   `yaml:"created_at"`
	Provenance  Provenance  `yaml:"provenance"`
	Links       []Link      `yaml:"links,omitempty"`
	Contexts    []string    `yaml:"contexts,omitempty"`
	Runtime     string      `yaml:"runtime,omitempty"`
	Trust       Trust       `yaml:"trust,omitempty"`
	Inputs      []string    `yaml:"inputs,omitempty"`
	Triggers    string      `yaml:"triggers,omitempty"`
}

// Node is a single unit of information in the Forest graph.
type Node struct {
	ID          string
	ContentType ContentType
	CreatedAt   time.Time
	Provenance  Provenance
	Links       []Link
	Contexts    []string

	// Executable node fields
	Runtime  string
	Trust    Trust
	Inputs   []string
	Triggers string

	// Body is the content after the YAML frontmatter block.
	Body string
}

// ParseFrom reads a node from the Markdown + YAML frontmatter format.
func ParseFrom(r io.Reader) (*Node, error) {
	var fm frontmatterFields
	rest, err := frontmatter.Parse(r, &fm)
	if err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}
	return &Node{
		ID:          fm.ID,
		ContentType: fm.ContentType,
		CreatedAt:   fm.CreatedAt,
		Provenance:  fm.Provenance,
		Links:       fm.Links,
		Contexts:    fm.Contexts,
		Runtime:     fm.Runtime,
		Trust:       fm.Trust,
		Inputs:      fm.Inputs,
		Triggers:    fm.Triggers,
		Body:        strings.TrimSpace(string(rest)),
	}, nil
}

// Encode serialises a node to the Markdown + YAML frontmatter format.
func (n *Node) Encode(w io.Writer) error {
	fm := frontmatterFields{
		ID:          n.ID,
		ContentType: n.ContentType,
		CreatedAt:   n.CreatedAt.UTC(),
		Provenance:  n.Provenance,
		Links:       n.Links,
		Contexts:    n.Contexts,
		Runtime:     n.Runtime,
		Trust:       n.Trust,
		Inputs:      n.Inputs,
		Triggers:    n.Triggers,
	}

	// Write YAML frontmatter block manually; adrg/frontmatter is read-only.
	_, err := fmt.Fprintf(w, "---\n%s---\n\n%s\n", marshalYAML(fm), n.Body)
	return err
}

// marshalYAML produces a minimal YAML representation of the frontmatter fields.
// We avoid importing a separate YAML encoder to keep dependencies lean; the
// fields are predictable and the format is simple enough to write directly.
func marshalYAML(fm frontmatterFields) string {
	var sb strings.Builder
	sb.WriteString("id: " + fm.ID + "\n")
	sb.WriteString("content_type: " + string(fm.ContentType) + "\n")
	sb.WriteString("created_at: " + fm.CreatedAt.Format(time.RFC3339) + "\n")
	sb.WriteString("provenance:\n")
	sb.WriteString("  source: " + string(fm.Provenance.Source) + "\n")
	if fm.Provenance.ImportedURL != "" {
		sb.WriteString("  imported_url: " + fm.Provenance.ImportedURL + "\n")
	}
	if len(fm.Links) > 0 {
		sb.WriteString("links:\n")
		for _, l := range fm.Links {
			sb.WriteString("  - type: " + string(l.Type) + "\n")
			sb.WriteString("    target: " + l.TargetID + "\n")
			if l.TargetURL != "" {
				sb.WriteString("    target_url: " + l.TargetURL + "\n")
			}
		}
	}
	if len(fm.Contexts) > 0 {
		sb.WriteString("contexts:\n")
		for _, c := range fm.Contexts {
			sb.WriteString("  - " + c + "\n")
		}
	}
	if fm.Runtime != "" {
		sb.WriteString("runtime: " + fm.Runtime + "\n")
	}
	if fm.Trust != "" {
		sb.WriteString("trust: " + string(fm.Trust) + "\n")
	}
	if len(fm.Inputs) > 0 {
		sb.WriteString("inputs:\n")
		for _, inp := range fm.Inputs {
			sb.WriteString("  - " + inp + "\n")
		}
	}
	if fm.Triggers != "" {
		sb.WriteString("triggers: " + fm.Triggers + "\n")
	}
	return sb.String()
}
