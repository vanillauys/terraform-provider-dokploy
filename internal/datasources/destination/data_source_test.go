package destination

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// Ambiguity and not-found are the two paths most likely to be got wrong, and
// neither needs a server: findByName is a pure function over the list
// destination.all returns.

func TestFindByName_Ambiguous(t *testing.T) {
	dests := []client.Destination{
		{DestinationID: "d1", Name: "backups"},
		{DestinationID: "d2", Name: "backups"},
	}

	got, err := findByName(dests, "backups")
	if err == nil {
		t.Fatalf("got %+v, want an error: two destinations share the name, so resolving to either is wrong", got)
	}
	if !strings.Contains(err.Error(), "2 destinations") {
		t.Errorf("error %q does not say how many matched; the operator cannot tell ambiguity from a typo", err)
	}
	if !strings.Contains(err.Error(), "backups") {
		t.Errorf("error %q does not name the string searched for", err)
	}
}

func TestFindByName_NotFound(t *testing.T) {
	dests := []client.Destination{{DestinationID: "d1", Name: "backups"}}

	if _, err := findByName(dests, "typo"); err == nil {
		t.Fatal("got nil error for a name that matches nothing, want an error")
	} else if !strings.Contains(err.Error(), "typo") {
		t.Errorf("error %q does not name the string searched for, so the typo is invisible without debug logging", err)
	}
}

// TestFindByName_Exact exists because a substring or prefix match is the
// other way this goes wrong, and it is covered by neither the ambiguity nor
// the not-found case.
func TestFindByName_Exact(t *testing.T) {
	dests := []client.Destination{
		{DestinationID: "d1", Name: "backups"},
		{DestinationID: "d2", Name: "backups-old"},
	}

	got, err := findByName(dests, "backups")
	if err != nil {
		t.Fatalf("findByName: %v", err)
	}
	if got.DestinationID != "d1" {
		t.Errorf("got %q, want d1 - a prefix match must not count as a match", got.DestinationID)
	}
}

// The data source must not model credentials. This pins that decision so a
// later "just add the access key, it is already on the wire" edit has to
// delete a test that says why not, rather than silently widening the blast
// radius of a shared backup target's secret.
//
// destination.one returns accessKey and secretAccessKey in cleartext, so
// nothing on the wire stops this from being added by accident.
func TestSchemaDoesNotExposeCredentials(t *testing.T) {
	var resp datasource.SchemaResponse
	(&destinationDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, &resp)

	for _, banned := range []string{"access_key", "secret_access_key"} {
		if _, found := resp.Schema.Attributes[banned]; found {
			t.Errorf("the schema exposes %q; consumers need only the id, and copying a shared target's credentials into every consumer's state widens their blast radius for no gain", banned)
		}
	}
}

// The model and the schema have to agree attribute-for-attribute, or
// State.Set fails at runtime rather than at build time. reflect over the
// tfsdk tags is the cheapest way to keep them in step.
func TestSchemaAndModelAgree(t *testing.T) {
	var resp datasource.SchemaResponse
	(&destinationDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, &resp)

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
