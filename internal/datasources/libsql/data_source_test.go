package libsql

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
// ListLibsqlByEnvironment returns.

// TestFindByNameErrorsOnAmbiguity is the brief's own test, verbatim: two
// records sharing a name must never resolve to refs[0] - Dokploy does not
// enforce unique service names within an environment.
func TestFindByNameErrorsOnAmbiguity(t *testing.T) {
	refs := []client.ServiceRef{
		{ID: "lib-1", Name: "edge"},
		{ID: "lib-2", Name: "edge"},
	}
	_, err := findByName(refs, "edge")
	if err == nil {
		t.Fatal("two records with the same name must be an error, never [0]")
	}
	if !strings.Contains(err.Error(), "2") {
		t.Errorf("error should name the match count, got %q", err)
	}
}

func TestFindByNameErrorsOnNotFound(t *testing.T) {
	refs := []client.ServiceRef{{ID: "lib-1", Name: "edge"}}

	if _, err := findByName(refs, "typo"); err == nil {
		t.Fatal("got nil error for a name that matches nothing, want an error")
	} else if !strings.Contains(err.Error(), "typo") {
		t.Errorf("error %q does not name the string searched for, so the typo is invisible without debug logging", err)
	}
}

// TestFindByNameReturnsExactMatch exists because a prefix match is the other
// way this goes wrong, and it is covered by neither the ambiguity nor the
// not-found case.
func TestFindByNameReturnsExactMatch(t *testing.T) {
	refs := []client.ServiceRef{
		{ID: "lib-1", Name: "edge"},
		{ID: "lib-2", Name: "edge-old"},
	}

	got, err := findByName(refs, "edge")
	if err != nil {
		t.Fatalf("findByName: %v", err)
	}
	if got.ID != "lib-1" {
		t.Errorf("got %q, want lib-1 - a prefix match must not count as a match", got.ID)
	}
}

// database_password is deliberately exposed here, unlike dokploy_destination:
// this is a single service's own password rather than a shared target's
// credential, and the five database-engine data sources already expose
// theirs. This test pins that decision so it does not regress by accident.
func TestSchemaExposesDatabasePassword(t *testing.T) {
	var resp datasource.SchemaResponse
	(&libsqlDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, &resp)

	attr, found := resp.Schema.Attributes["database_password"]
	if !found {
		t.Fatal("database_password is missing from the schema")
	}
	if !attr.IsComputed() {
		t.Error("database_password must be Computed - a data source only reads")
	}
	if !attr.IsSensitive() {
		t.Error("database_password must be Sensitive")
	}
}

// The model and the schema have to agree attribute-for-attribute, or
// State.Set fails at runtime rather than at build time. reflect over the
// tfsdk tags is the cheapest way to keep them in step.
func TestSchemaAndModelAgree(t *testing.T) {
	var resp datasource.SchemaResponse
	(&libsqlDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, &resp)

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
