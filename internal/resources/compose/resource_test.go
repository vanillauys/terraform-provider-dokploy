package compose

import (
	"context"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// nullObjectValue builds an all-null tftypes value matching the schema, the
// starting point tfsdk.State needs before anything can be written into it.
func nullObjectValue(t *testing.T, s schema.Schema) tftypes.Value {
	t.Helper()
	return tftypes.NewValue(s.Type().TerraformType(context.Background()), nil)
}

func testSchema(t *testing.T) schema.Schema {
	t.Helper()
	var resp resource.SchemaResponse
	(&composeResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("building the schema: %v", resp.Diagnostics)
	}
	return resp.Schema
}

// The schema and the model must agree attribute-for-attribute, or State.Set
// fails at apply time rather than at build time. This is the cheapest place
// to catch a renamed tfsdk tag, and it names the offending side rather than
// failing with the framework's generic conversion error.
func TestSchemaAndModelAgree(t *testing.T) {
	s := testSchema(t)

	typ := reflect.TypeOf(resourceModel{})
	tags := make(map[string]bool, typ.NumField())
	for i := range typ.NumField() {
		tags[typ.Field(i).Tag.Get("tfsdk")] = true
	}

	for name := range s.Attributes {
		if !tags[name] {
			t.Errorf("schema attribute %q has no tfsdk field on resourceModel", name)
		}
	}
	for tag := range tags {
		if _, found := s.Attributes[tag]; !found {
			t.Errorf("resourceModel field tagged %q has no schema attribute", tag)
		}
	}
}

// The nested source blocks have their own tfsdk structs, and a renamed tag
// inside one is just as fatal and less visible.
func TestNestedSourceBlocksMatchTheirSchemas(t *testing.T) {
	s := testSchema(t)

	for _, tc := range []struct {
		attr  string
		model any
	}{
		{"github", githubSource{}},
		{"git", gitSource{}},
		{"raw", rawSource{}},
	} {
		nested, ok := s.Attributes[tc.attr].(schema.SingleNestedAttribute)
		if !ok {
			t.Errorf("%s is not a SingleNestedAttribute", tc.attr)
			continue
		}
		typ := reflect.TypeOf(tc.model)
		tags := make(map[string]bool, typ.NumField())
		for i := range typ.NumField() {
			tags[typ.Field(i).Tag.Get("tfsdk")] = true
		}
		for name := range nested.Attributes {
			if !tags[name] {
				t.Errorf("%s.%s has no tfsdk field on %s", tc.attr, name, typ.Name())
			}
		}
		for tag := range tags {
			if _, found := nested.Attributes[tag]; !found {
				t.Errorf("%s field tagged %q has no schema attribute under %s", typ.Name(), tag, tc.attr)
			}
		}
	}
}

// The three source blocks are modelled as Go pointer structs rather than
// types.Object. A nil pointer must round-trip as a null object, or a config
// that omits two of the three blocks cannot be written to state at all.
//
// This is the one structural assumption compose makes that dokploy_
// application does not - application uses types.Object throughout - so it is
// worth proving rather than inferring from the framework's docs.
func TestSourceBlocksRoundTripThroughState(t *testing.T) {
	s := testSchema(t)
	ctx := context.Background()

	m := resourceModel{
		ID:              types.StringValue("c1"),
		Name:            types.StringValue("web"),
		EnvironmentID:   types.StringValue("env1"),
		AppName:         types.StringValue("web-abc"),
		ComposeType:     types.StringValue("docker-compose"),
		Status:          types.StringValue("idle"),
		CreatedAt:       types.StringValue("2026-07-29T00:00:00.000Z"),
		WatchPaths:      types.ListNull(types.StringType),
		ServiceNetworks: types.SetNull(types.ObjectType{AttrTypes: serviceNetworkAttrTypes}),
		Git: &gitSource{
			URL:      types.StringValue("git@example.com:acme/site.git"),
			Branch:   types.StringValue("main"),
			SSHKeyID: types.StringNull(),
		},
	}

	state := tfsdk.State{Raw: nullObjectValue(t, s), Schema: s}
	if diags := state.Set(ctx, &m); diags.HasError() {
		t.Fatalf("writing a model with only the git block set: %v", diags)
	}

	var back resourceModel
	if diags := state.Get(ctx, &back); diags.HasError() {
		t.Fatalf("reading it back: %v", diags)
	}

	if back.Git == nil {
		t.Fatal("git block came back nil after a round trip")
	}
	if back.Git.URL.ValueString() != "git@example.com:acme/site.git" {
		t.Errorf("git.url = %q", back.Git.URL.ValueString())
	}
	if back.Github != nil {
		t.Errorf("github block = %+v, want nil: an unset source block must round-trip as null", back.Github)
	}
	if back.Raw != nil {
		t.Errorf("raw block = %+v, want nil", back.Raw)
	}
}

// status must NOT carry UseStateForUnknown. It is server-mutable - a deploy
// moves it - so pinning the prior value as a known plan value makes core
// reject the apply with "Provider produced inconsistent result after apply".
// application/resource.go carries the same rule as a prose comment; here it
// is enforced.
func TestStatusHasNoPlanModifiers(t *testing.T) {
	attr, ok := testSchema(t).Attributes["status"].(schema.StringAttribute)
	if !ok {
		t.Fatal("status is not a StringAttribute")
	}
	if len(attr.PlanModifiers) != 0 {
		t.Errorf("status has %d plan modifiers, want none: it is server-mutable, and pinning it fails the apply with an inconsistent result", len(attr.PlanModifiers))
	}
}

// The source blocks must be Optional and NOT Computed. An Optional+Computed
// nested attribute makes the framework mark every config-null Computed
// attribute unknown, producing perpetual "(known after apply)" on id,
// created_at and status - the trap tfutil's package comment documents and
// application's ModifyPlan works around at length.
func TestSourceBlocksAreNotComputed(t *testing.T) {
	s := testSchema(t)
	for _, name := range []string{"github", "git", "raw"} {
		attr, ok := s.Attributes[name].(schema.SingleNestedAttribute)
		if !ok {
			t.Errorf("%s is not a SingleNestedAttribute", name)
			continue
		}
		if !attr.Optional {
			t.Errorf("%s is not Optional", name)
		}
		if attr.Computed {
			t.Errorf("%s is Optional+Computed, which marks every config-null Computed attribute unknown on every plan", name)
		}
	}
}
