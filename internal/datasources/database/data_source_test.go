package database

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	resourcedb "github.com/vanillauys/terraform-provider-dokploy/internal/resources/database"
)

// TestSchemaAttributes_TwoCredentialAttrs checks the postgres-shaped Kind's
// generated data-source schema carries both credential attrs as plain
// Computed strings with the exact shipped descriptions — the shape the
// byte-identical-docs gate (docs/data-sources/postgres.md) depends on.
func TestSchemaAttributes_TwoCredentialAttrs(t *testing.T) {
	k := resourcedb.PostgresKind(nil)
	attrs := schemaAttributes(k)

	for _, name := range []string{
		"id", "name", "environment_id", "app_name", "database_name",
		"database_user", "docker_image", "external_port", "status", "created_at",
	} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("expected attribute %q in schema, not found", name)
		}
	}
	// This data source never exposes these — they belong to the resource
	// side only (docs/data-sources/postgres.md has no description, env,
	// server_id, or database_password attribute).
	for _, name := range []string{"description", "env", "server_id", "database_password"} {
		if _, ok := attrs[name]; ok {
			t.Errorf("data-source schema should not define %q", name)
		}
	}

	dbName, ok := attrs["database_name"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("database_name is not a schema.StringAttribute: %T", attrs["database_name"])
	}
	if !dbName.Computed || dbName.Optional || dbName.Required || dbName.Sensitive {
		t.Error("database_name should be Computed-only, never Optional/Required/Sensitive")
	}
	if dbName.Description != "Database name." {
		t.Errorf("unexpected database_name description: %q", dbName.Description)
	}

	dbUser, ok := attrs["database_user"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("database_user is not a schema.StringAttribute: %T", attrs["database_user"])
	}
	if dbUser.Description != "Database user." {
		t.Errorf("unexpected database_user description: %q", dbUser.Description)
	}

	id, ok := attrs["id"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("id is not a schema.StringAttribute: %T", attrs["id"])
	}
	if id.Description != "Postgres service id. Set either this or both `environment_id` and `name`." {
		t.Errorf("unexpected id description: %q", id.Description)
	}

	name, ok := attrs["name"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("name is not a schema.StringAttribute: %T", attrs["name"])
	}
	if name.Description != "Exact postgres service name, searched within `environment_id`. Errors when zero or multiple postgres services match." {
		t.Errorf("unexpected name description: %q", name.Description)
	}
}

// TestSchemaAttributes_ZeroCredentialAttrs mirrors the resource engine's
// same-named test (internal/resources/database/model_test.go): a
// redis-shaped Kind (per internal/client/doc.go, redis has "NO
// databaseUser, NO databaseName") must still build a valid data-source
// schema — the uniform read-only set, and nothing beyond that.
func TestSchemaAttributes_ZeroCredentialAttrs(t *testing.T) {
	k := resourcedb.Kind{
		Name:            "redis",
		HumanName:       "Redis",
		ShortName:       "Redis",
		CredentialAttrs: nil,
	}
	attrs := schemaAttributes(k)

	for _, name := range []string{
		"id", "name", "environment_id", "app_name", "docker_image",
		"external_port", "status", "created_at",
	} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("expected uniform attribute %q in a zero-credential-attr schema, not found", name)
		}
	}
	for _, name := range []string{"database_name", "database_user"} {
		if _, ok := attrs[name]; ok {
			t.Errorf("zero-credential-attr schema should not define %q", name)
		}
	}

	id, ok := attrs["id"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("id is not a schema.StringAttribute: %T", attrs["id"])
	}
	if id.Description != "Redis service id. Set either this or both `environment_id` and `name`." {
		t.Errorf("unexpected id description: %q", id.Description)
	}
}

func TestHumanizeAttrName(t *testing.T) {
	cases := map[string]string{
		"database_name":          "Database name.",
		"database_user":          "Database user.",
		"database_root_password": "Database root password.",
	}
	for tfName, want := range cases {
		if got := humanizeAttrName(tfName); got != want {
			t.Errorf("humanizeAttrName(%q) = %q, want %q", tfName, got, want)
		}
	}
}

// TestFindByName mirrors internal/client/environment_test.go's
// TestFindServiceByName exactly, retargeted to resourcedb.Object: Dokploy
// allows two services of the same kind in one environment to share a name,
// so a name lookup must refuse an ambiguous match rather than silently
// taking the first.
func TestFindByName(t *testing.T) {
	objs := []resourcedb.Object{
		{ID: "a1", Name: "frontend"},
		{ID: "a2", Name: "shared"},
		{ID: "a3", Name: "shared"},
	}

	got, err := findByName(objs, "frontend", "postgres")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "a1" {
		t.Errorf("id = %q, want a1", got)
	}

	if _, err := findByName(objs, "shared", "postgres"); err == nil {
		t.Error("two postgres services named shared must be an error, not a silent pick")
	} else if !strings.Contains(err.Error(), "multiple") {
		t.Errorf("error %q should mention multiple matches", err)
	}

	if _, err := findByName(objs, "absent", "postgres"); err == nil {
		t.Error("no match must be an error")
	}

	// Synthetic fixture, not an observed server behavior: proves the
	// sentinel's own logic (a *string, not a string compared against ""),
	// not a claim that Dokploy actually returns an empty service ID.
	emptyIDDup := []resourcedb.Object{
		{ID: "", Name: "dup"},
		{ID: "b2", Name: "dup"},
	}
	if _, err := findByName(emptyIDDup, "dup", "postgres"); err == nil {
		t.Error("two postgres services named dup must be an error even when the first match has an empty id")
	} else if !strings.Contains(err.Error(), "multiple") {
		t.Errorf("error %q should mention multiple matches", err)
	}
}

// TestApplyObject_TwoCredentialAttrs exercises applyObject (the
// data-source equivalent of internal/resources/database/model.go's
// flatten) against a Kind with two credential attrs (postgres-shaped).
func TestApplyObject_TwoCredentialAttrs(t *testing.T) {
	k := resourcedb.PostgresKind(nil)
	port := int64(5432)
	obj := &resourcedb.Object{
		ID:                "pg-1",
		Name:              "mydb",
		AppName:           "app-1",
		EnvironmentID:     "env-1",
		DockerImage:       "postgres:16-alpine",
		ApplicationStatus: "done",
		CreatedAt:         "2026-01-01T00:00:00Z",
		ExternalPort:      &port,
		// DatabasePassword deliberately set to prove applyObject never
		// copies it into the data-source model.
		DatabasePassword: "hunter2",
		Credentials: map[string]string{
			"database_name": "mydb",
			"database_user": "myuser",
		},
	}
	var m genericModel
	applyObject(k, obj, &m)

	if got := m.ID.ValueString(); got != "pg-1" {
		t.Errorf("ID = %q, want pg-1", got)
	}
	if got := m.Name.ValueString(); got != "mydb" {
		t.Errorf("Name = %q, want mydb", got)
	}
	if got := m.AppName.ValueString(); got != "app-1" {
		t.Errorf("AppName = %q, want app-1", got)
	}
	if got := m.EnvironmentID.ValueString(); got != "env-1" {
		t.Errorf("EnvironmentID = %q, want env-1", got)
	}
	if got := m.DockerImage.ValueString(); got != "postgres:16-alpine" {
		t.Errorf("DockerImage = %q, want postgres:16-alpine", got)
	}
	if got := m.ExternalPort.ValueInt64(); got != port {
		t.Errorf("ExternalPort = %d, want %d", got, port)
	}
	if got := m.Status.ValueString(); got != "done" {
		t.Errorf("Status = %q, want done", got)
	}
	if got := m.CreatedAt.ValueString(); got != "2026-01-01T00:00:00Z" {
		t.Errorf("CreatedAt = %q, want 2026-01-01T00:00:00Z", got)
	}
	if got := m.Credentials["database_name"].ValueString(); got != "mydb" {
		t.Errorf("Credentials[database_name] = %q, want mydb", got)
	}
	if got := m.Credentials["database_user"].ValueString(); got != "myuser" {
		t.Errorf("Credentials[database_user] = %q, want myuser", got)
	}
}

// TestApplyObject_MissingCredentialGoesNull guards the defensive fallback
// in applyObject: a CredentialAttr absent from Object.Credentials (which a
// correct adapter should never produce, but the generic engine must not
// panic on) flattens to a null Terraform value rather than a zero-value
// types.String.
func TestApplyObject_MissingCredentialGoesNull(t *testing.T) {
	k := resourcedb.PostgresKind(nil)
	obj := &resourcedb.Object{
		ID:          "pg-1",
		Credentials: map[string]string{},
	}
	var m genericModel
	applyObject(k, obj, &m)
	if !m.Credentials["database_name"].IsNull() {
		t.Errorf("expected database_name null when absent from Object.Credentials, got %v", m.Credentials["database_name"])
	}
	if !m.Credentials["database_user"].IsNull() {
		t.Errorf("expected database_user null when absent from Object.Credentials, got %v", m.Credentials["database_user"])
	}
}

// TestApplyObject_NilExternalPortGoesNull asserts the pointer round-trip
// through types.Int64PointerValue: a nil ExternalPort must flatten to a
// null Terraform value, not zero.
func TestApplyObject_NilExternalPortGoesNull(t *testing.T) {
	k := resourcedb.PostgresKind(nil)
	obj := &resourcedb.Object{ID: "pg-1", Credentials: map[string]string{}}
	var m genericModel
	applyObject(k, obj, &m)
	if !m.ExternalPort.IsNull() {
		t.Errorf("expected ExternalPort null when Object.ExternalPort is nil, got %v", m.ExternalPort)
	}
}
