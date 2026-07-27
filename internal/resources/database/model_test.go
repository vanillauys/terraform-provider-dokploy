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
	setComputed(Kind{}, obj, &m)

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

// TestSetComputed_RefreshesComputedCredential guards the review finding on
// this task: setComputed used to leave every CredentialAttr untouched, so a
// Computed one (mysql/mariadb's server-generated databaseRootPassword, the
// stated motivation for CredentialAttr.Computed) would carry its Create-time
// Unknown plan value (Optional+Computed+UseStateForUnknown, no prior state to
// fall back to) straight into committed state — which Terraform core rejects
// with "Provider produced inconsistent result after apply." setComputed is
// exactly what Create/Update call after fetching the server object
// (resource.go), so this reproduces the Create/Update path's use of it
// directly: start from the Unknown plan value, call setComputed with the
// server's Object, and assert the result is neither Unknown nor null but the
// server's actual value.
func TestSetComputed_RefreshesComputedCredential(t *testing.T) {
	k := Kind{
		Name:      "mysql",
		HumanName: "MySQL",
		CredentialAttrs: []CredentialAttr{
			{TFName: "database_root_password", Computed: true},
		},
	}
	obj := &Object{
		ID:                "mysql-1",
		AppName:           "app-1",
		DockerImage:       "mysql:8",
		ApplicationStatus: "done",
		CreatedAt:         "2026-01-01T00:00:00Z",
		Credentials: map[string]string{
			"database_root_password": "server-generated-secret",
		},
	}
	m := genericModel{
		Credentials: map[string]types.String{
			// Simulates the plan value getModel would read on Create when the
			// attribute is omitted from config: Optional+Computed with no
			// prior state, so the framework marks it Unknown.
			"database_root_password": types.StringUnknown(),
		},
	}
	setComputed(k, obj, &m)

	got := m.Credentials["database_root_password"]
	if got.IsUnknown() {
		t.Fatal("Computed credential attr left Unknown after setComputed: Terraform core will reject this apply with \"Provider produced inconsistent result after apply\"")
	}
	if got.IsNull() {
		t.Fatal("Computed credential attr is null after setComputed, want the server-refreshed value")
	}
	if v := got.ValueString(); v != "server-generated-secret" {
		t.Errorf("Credentials[database_root_password] = %q, want server-generated-secret", v)
	}
}

// TestSetComputed_LeavesNonComputedCredentialAlone is the fix's other half:
// a non-Computed CredentialAttr (postgres's database_name/database_user) is a
// plain user-supplied config value, not a server-computed one. setComputed
// must not clobber it with the server's value, or postgres's shipped state
// handling (zero Computed CredentialAttrs today) would silently change.
func TestSetComputed_LeavesNonComputedCredentialAlone(t *testing.T) {
	k := PostgresKind(nil)
	obj := &Object{
		ID: "pg-1",
		Credentials: map[string]string{
			// Deliberately different from the plan values below: if
			// setComputed touched non-Computed attrs, this test would catch
			// it clobbering the user's config with these instead.
			"database_name": "server-side-name",
			"database_user": "server-side-user",
		},
	}
	m := genericModel{
		Credentials: map[string]types.String{
			"database_name": types.StringValue("mydb"),
			"database_user": types.StringValue("myuser"),
		},
	}
	setComputed(k, obj, &m)

	if got := m.Credentials["database_name"].ValueString(); got != "mydb" {
		t.Errorf("non-Computed database_name was clobbered by setComputed: got %q, want mydb", got)
	}
	if got := m.Credentials["database_user"].ValueString(); got != "myuser" {
		t.Errorf("non-Computed database_user was clobbered by setComputed: got %q, want myuser", got)
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
