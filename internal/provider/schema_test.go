package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

var updateSnapshot = flag.Bool("update", false, "rewrite testdata/snapSchema.json from the current provider schema")

// snapshot is the part of the provider schema that the semver promise
// covers: the provider block and every resource, data source, ephemeral
// resource, and function. The compact types below carry the schema only:
// descriptions are documentation, which the docs check guards, so a wording
// change leaves the snapshot untouched, and empty fields are omitted.
type snapshot struct {
	Provider           *snapSchema                    `json:"provider"`
	Resources          map[string]*snapSchema         `json:"resources"`
	DataSources        map[string]*snapSchema         `json:"data_sources"`
	EphemeralResources map[string]*snapSchema         `json:"ephemeral_resources,omitempty"`
	Functions          map[string]*tfprotov6.Function `json:"functions,omitempty"`
}

type snapSchema struct {
	Version int64      `json:"version"`
	Block   *snapBlock `json:"block"`
}

type snapBlock struct {
	Attributes []snapAttribute   `json:"attributes,omitempty"`
	Blocks     []snapNestedBlock `json:"blocks,omitempty"`
	Deprecated bool              `json:"deprecated,omitempty"`
}

type snapNestedBlock struct {
	TypeName string     `json:"type_name"`
	Nesting  string     `json:"nesting"`
	MinItems int64      `json:"min_items,omitempty"`
	MaxItems int64      `json:"max_items,omitempty"`
	Block    *snapBlock `json:"block"`
}

type snapAttribute struct {
	Name       string      `json:"name"`
	Type       string      `json:"type,omitempty"`
	Nested     *snapObject `json:"nested,omitempty"`
	Required   bool        `json:"required,omitempty"`
	Optional   bool        `json:"optional,omitempty"`
	Computed   bool        `json:"computed,omitempty"`
	Sensitive  bool        `json:"sensitive,omitempty"`
	WriteOnly  bool        `json:"write_only,omitempty"`
	Deprecated bool        `json:"deprecated,omitempty"`
}

type snapObject struct {
	Nesting    string          `json:"nesting"`
	Attributes []snapAttribute `json:"attributes"`
}

// TestSchemaSnapshot pins the provider schema to testdata/schema.json. A
// schema change fails here until the snapshot is regenerated:
//
//	go test ./internal/provider -run TestSchemaSnapshot -update
//
// Review the diff of that file in the pull request. A removed attribute, a
// type change, or an optional attribute that became required is a breaking
// change and needs a major version; an added optional attribute needs a
// minor version.
func TestSchemaSnapshot(t *testing.T) {
	server := providerserver.NewProtocol6(New("test")())()
	resp, err := server.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("GetProviderSchema: %v", err)
	}
	for _, d := range resp.Diagnostics {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			t.Fatalf("schema diagnostic: %s: %s", d.Summary, d.Detail)
		}
	}
	snap := snapshot{
		Provider:           normalize(resp.Provider),
		Resources:          normalizeAll(resp.ResourceSchemas),
		DataSources:        normalizeAll(resp.DataSourceSchemas),
		EphemeralResources: normalizeAll(resp.EphemeralResourceSchemas),
		Functions:          resp.Functions,
	}
	got, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got = append(got, '\n')

	path := filepath.Join("testdata", "schema.json")
	if *updateSnapshot {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (run with -update to create it)", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("the provider snapSchema differs from %s.\n"+
			"Run `go test ./internal/provider -run TestSchemaSnapshot -update`, then review the diff:\n"+
			"a removed snapAttribute, a type change, or an optional snapAttribute that became required is a breaking change.", path)
	}
}

func normalizeAll(in map[string]*tfprotov6.Schema) map[string]*snapSchema {
	out := make(map[string]*snapSchema, len(in))
	for name, s := range in {
		out[name] = normalize(s)
	}
	return out
}

// normalize sorts the attributes and blocks by name, so the JSON is stable.
func normalize(s *tfprotov6.Schema) *snapSchema {
	if s == nil {
		return nil
	}
	return &snapSchema{Version: s.Version, Block: normalizeBlock(s.Block)}
}

func normalizeBlock(b *tfprotov6.SchemaBlock) *snapBlock {
	if b == nil {
		return nil
	}
	out := &snapBlock{Deprecated: b.Deprecated}
	for _, a := range b.Attributes {
		out.Attributes = append(out.Attributes, normalizeAttribute(a))
	}
	sort.Slice(out.Attributes, func(i, j int) bool { return out.Attributes[i].Name < out.Attributes[j].Name })
	for _, nb := range b.BlockTypes {
		out.Blocks = append(out.Blocks, snapNestedBlock{
			TypeName: nb.TypeName,
			Nesting:  fmt.Sprint(nb.Nesting),
			MinItems: nb.MinItems,
			MaxItems: nb.MaxItems,
			Block:    normalizeBlock(nb.Block),
		})
	}
	sort.Slice(out.Blocks, func(i, j int) bool { return out.Blocks[i].TypeName < out.Blocks[j].TypeName })
	return out
}

func normalizeAttribute(a *tfprotov6.SchemaAttribute) snapAttribute {
	out := snapAttribute{
		Name:       a.Name,
		Required:   a.Required,
		Optional:   a.Optional,
		Computed:   a.Computed,
		Sensitive:  a.Sensitive,
		WriteOnly:  a.WriteOnly,
		Deprecated: a.Deprecated,
	}
	if a.Type != nil {
		out.Type = a.Type.String()
	}
	if a.NestedType != nil {
		nt := &snapObject{Nesting: fmt.Sprint(a.NestedType.Nesting)}
		for _, na := range a.NestedType.Attributes {
			nt.Attributes = append(nt.Attributes, normalizeAttribute(na))
		}
		sort.Slice(nt.Attributes, func(i, j int) bool { return nt.Attributes[i].Name < nt.Attributes[j].Name })
		out.Nested = nt
	}
	return out
}
