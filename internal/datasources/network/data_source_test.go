package network

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func strp(s string) *string { return &s }

func TestFindByName(t *testing.T) {
	networks := []client.Network{
		{NetworkID: "n1", Name: "edge"},
		{NetworkID: "n2", Name: "backend"},
		{NetworkID: "n3", Name: "backend", ServerID: strp("srv1")},
	}

	if got, err := findByName(networks, "edge", nil); err != nil || got.NetworkID != "n1" {
		t.Errorf("edge = %+v, %v", got, err)
	}
	if _, err := findByName(networks, "missing", nil); err == nil || !strings.Contains(err.Error(), "no network named") {
		t.Errorf("missing: err = %v, want a no-match error", err)
	}
	// Never take [0] on a duplicate name.
	if _, err := findByName(networks, "backend", nil); err == nil || !strings.Contains(err.Error(), "2 networks") {
		t.Errorf("duplicate: err = %v, want a multi-match error", err)
	}
	// server_id narrows a duplicate to one.
	if got, err := findByName(networks, "backend", strp("srv1")); err != nil || got.NetworkID != "n3" {
		t.Errorf("narrowed = %+v, %v", got, err)
	}
}

// The data source must not model ipam. This pins that decision so a later
// "just add it back, it is already on the wire" edit has to delete a test
// that says why not, rather than silently widening a shared network's
// blast radius. Mirrors destination's TestSchemaDoesNotExposeCredentials.
func TestSchemaDoesNotExposeIPAM(t *testing.T) {
	var resp datasource.SchemaResponse
	(&networkDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, &resp)

	if _, found := resp.Schema.Attributes["ipam"]; found {
		t.Error("the schema exposes \"ipam\"; a consumer needs only the id, and copying a shared network's address pools into every consumer's state widens their blast radius for no gain")
	}
}

// The model and the schema have to agree attribute-for-attribute, or
// State.Set fails at runtime rather than at build time. reflect over the
// tfsdk tags is the cheapest way to keep them in step. Mirrors
// destination's TestSchemaAndModelAgree.
func TestSchemaAndModelAgree(t *testing.T) {
	var resp datasource.SchemaResponse
	(&networkDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, &resp)

	typ := reflect.TypeOf(model{})
	tags := make(map[string]bool, typ.NumField())
	for i := range typ.NumField() {
		tags[typ.Field(i).Tag.Get("tfsdk")] = true
	}

	for name := range resp.Schema.Attributes {
		if !tags[name] {
			t.Errorf("schema attribute %q has no tfsdk field on model", name)
		}
	}
	for tag := range tags {
		if _, found := resp.Schema.Attributes[tag]; !found {
			t.Errorf("model field tagged %q has no schema attribute", tag)
		}
	}
}
