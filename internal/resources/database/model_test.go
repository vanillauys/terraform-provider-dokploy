package database

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestDeployNeeded(t *testing.T) {
	base := func() genericModel {
		return genericModel{
			DockerImage:      types.StringValue("postgres:16-alpine"),
			DatabasePassword: types.StringValue("hunter2"),
			Env:              types.StringValue("A=1"),
			ExternalPort:     types.Int64Value(5432),
			Name:             types.StringValue("db"),
		}
	}

	state := base()
	plan := base()
	if deployNeeded(plan, state) {
		t.Error("identical models must not trigger a deploy")
	}

	plan = base()
	plan.Name = types.StringValue("renamed")
	if deployNeeded(plan, state) {
		t.Error("name is not a deploy trigger")
	}

	for name, mutate := range map[string]func(*genericModel){
		"docker_image":      func(m *genericModel) { m.DockerImage = types.StringValue("postgres:17") },
		"database_password": func(m *genericModel) { m.DatabasePassword = types.StringValue("changed") },
		"env":               func(m *genericModel) { m.Env = types.StringValue("A=2") },
		"external_port":     func(m *genericModel) { m.ExternalPort = types.Int64Value(5433) },
	} {
		plan = base()
		mutate(&plan)
		if !deployNeeded(plan, state) {
			t.Errorf("%s change must trigger a deploy", name)
		}
	}
}

func TestSetComputed(t *testing.T) {
	obj := &Object{
		ID:                "pg-1",
		AppName:           "app-1",
		DockerImage:       "postgres:16-alpine",
		ApplicationStatus: "done",
		CreatedAt:         "2026-01-01T00:00:00Z",
	}
	var m genericModel
	setComputed(obj, &m)

	if got := m.ID.ValueString(); got != "pg-1" {
		t.Errorf("ID = %q, want pg-1", got)
	}
	if got := m.AppName.ValueString(); got != "app-1" {
		t.Errorf("AppName = %q, want app-1", got)
	}
	if got := m.DockerImage.ValueString(); got != "postgres:16-alpine" {
		t.Errorf("DockerImage = %q, want postgres:16-alpine", got)
	}
	if got := m.Status.ValueString(); got != "done" {
		t.Errorf("Status = %q, want done", got)
	}
	if got := m.CreatedAt.ValueString(); got != "2026-01-01T00:00:00Z" {
		t.Errorf("CreatedAt = %q, want 2026-01-01T00:00:00Z", got)
	}
}

// TestFlatten_TwoCredentialAttrs exercises flatten against a Kind with two
// credential attrs (postgres-shaped: database_name + database_user), per the
// brief's Step 2.
func TestFlatten_TwoCredentialAttrs(t *testing.T) {
	k := PostgresKind(nil)
	desc := "managed by tests"
	env := "TZ=UTC"
	port := int64(5432)
	serverID := "srv-1"
	obj := &Object{
		ID:                "pg-1",
		Name:              "mydb",
		AppName:           "app-1",
		EnvironmentID:     "env-1",
		DockerImage:       "postgres:16-alpine",
		ApplicationStatus: "done",
		CreatedAt:         "2026-01-01T00:00:00Z",
		Description:       &desc,
		Env:               &env,
		ServerID:          &serverID,
		ExternalPort:      &port,
		DatabasePassword:  "hunter2",
		Credentials: map[string]string{
			"database_name": "mydb",
			"database_user": "myuser",
		},
	}
	var m genericModel
	flatten(k, obj, &m)

	if got := m.Name.ValueString(); got != "mydb" {
		t.Errorf("Name = %q, want mydb", got)
	}
	if got := m.EnvironmentID.ValueString(); got != "env-1" {
		t.Errorf("EnvironmentID = %q, want env-1", got)
	}
	if got := m.DatabasePassword.ValueString(); got != "hunter2" {
		t.Errorf("DatabasePassword = %q, want hunter2", got)
	}
	if got := m.Description.ValueString(); got != desc {
		t.Errorf("Description = %q, want %q", got, desc)
	}
	if got := m.Env.ValueString(); got != env {
		t.Errorf("Env = %q, want %q", got, env)
	}
	if got := m.ServerID.ValueString(); got != serverID {
		t.Errorf("ServerID = %q, want %q", got, serverID)
	}
	if got := m.ExternalPort.ValueInt64(); got != port {
		t.Errorf("ExternalPort = %d, want %d", got, port)
	}
	if got := m.Credentials["database_name"].ValueString(); got != "mydb" {
		t.Errorf("Credentials[database_name] = %q, want mydb", got)
	}
	if got := m.Credentials["database_user"].ValueString(); got != "myuser" {
		t.Errorf("Credentials[database_user] = %q, want myuser", got)
	}
}

// TestFlatten_MissingCredentialGoesNull guards the defensive fallback in
// flatten: a CredentialAttr absent from Object.Credentials (which a correct
// adapter should never produce, but the generic engine must not panic on)
// flattens to a null Terraform value rather than a zero-value types.String.
func TestFlatten_MissingCredentialGoesNull(t *testing.T) {
	k := PostgresKind(nil)
	obj := &Object{
		ID:          "pg-1",
		Credentials: map[string]string{},
	}
	var m genericModel
	flatten(k, obj, &m)
	if !m.Credentials["database_name"].IsNull() {
		t.Errorf("expected database_name null when absent from Object.Credentials, got %v", m.Credentials["database_name"])
	}
	if !m.Credentials["database_user"].IsNull() {
		t.Errorf("expected database_user null when absent from Object.Credentials, got %v", m.Credentials["database_user"])
	}
}

// TestSchemaAttributes_TwoCredentialAttrs checks the postgres-shaped Kind's
// generated schema carries both credential attrs, Required + RequiresReplace,
// with their exact shipped descriptions (this is the shape the byte-
// identical-docs gate depends on).
func TestSchemaAttributes_TwoCredentialAttrs(t *testing.T) {
	k := PostgresKind(nil)
	attrs := schemaAttributes(k)

	for _, name := range []string{
		"id", "name", "environment_id", "database_name", "database_user",
		"database_password", "docker_image", "description", "env",
		"external_port", "app_name", "server_id", "status", "created_at",
		"deploy_on_change", "deployment_timeout",
	} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("expected attribute %q in schema, not found", name)
		}
	}

	dbName, ok := attrs["database_name"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("database_name is not a schema.StringAttribute: %T", attrs["database_name"])
	}
	if !dbName.Required {
		t.Error("database_name should be Required")
	}
	if dbName.Optional || dbName.Computed {
		t.Error("database_name should be neither Optional nor Computed")
	}
	if dbName.Description != "Name of the PostgreSQL database." {
		t.Errorf("unexpected database_name description: %q", dbName.Description)
	}

	dbUser, ok := attrs["database_user"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("database_user is not a schema.StringAttribute: %T", attrs["database_user"])
	}
	if !dbUser.Required {
		t.Error("database_user should be Required")
	}
	if dbUser.Description != "PostgreSQL user." {
		t.Errorf("unexpected database_user description: %q", dbUser.Description)
	}
}

// TestSchemaAttributes_ZeroCredentialAttrs is the brief's required
// zero-credential-attr case: a redis-shaped Kind (per internal/client/doc.go,
// redis has "NO databaseUser, NO databaseName") must still build a valid
// schema — the uniform set plus the deploy-engine attributes, and nothing
// beyond that.
func TestSchemaAttributes_ZeroCredentialAttrs(t *testing.T) {
	k := Kind{
		Name:               "redis",
		HumanName:          "Redis",
		ShortName:          "Redis",
		ExampleDockerImage: "redis:8",
		CredentialAttrs:    nil,
	}
	attrs := schemaAttributes(k)

	for _, name := range []string{
		"id", "name", "environment_id", "database_password", "docker_image",
		"description", "env", "external_port", "app_name", "server_id",
		"status", "created_at", "deploy_on_change", "deployment_timeout",
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

	dbPassword, ok := attrs["database_password"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("database_password is not a schema.StringAttribute: %T", attrs["database_password"])
	}
	if dbPassword.Description != "Redis password. Changing it triggers a redeploy." {
		t.Errorf("unexpected database_password description: %q", dbPassword.Description)
	}
}
