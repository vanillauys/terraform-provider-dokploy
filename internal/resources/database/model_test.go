package database

import (
	"context"
	"fmt"
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
	if deployNeeded(Kind{}, plan, state) {
		t.Error("identical models must not trigger a deploy")
	}

	plan = base()
	plan.Name = types.StringValue("renamed")
	if deployNeeded(Kind{}, plan, state) {
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
		if !deployNeeded(Kind{}, plan, state) {
			t.Errorf("%s change must trigger a deploy", name)
		}
	}
}

// TestDeployNeeded_CredentialAttr pins wave-2 task 5's addition:
// CredentialAttr.DeployTrigger, added after live evidence that
// mysql.update alone does not propagate a changed databaseRootPassword to
// the running container — only a following Deploy does (see DeployTrigger's
// doc comment in kind.go). A credential attribute changing must trigger a
// deploy when DeployTrigger is set, and must NOT when it isn't (postgres's
// database_name/database_user default, and the zero value generally).
func TestDeployNeeded_CredentialAttr(t *testing.T) {
	k := Kind{
		CredentialAttrs: []CredentialAttr{
			{TFName: "database_root_password", Computed: true, DeployTrigger: true},
			{TFName: "database_name", RequiresReplace: true}, // DeployTrigger left false
		},
	}
	base := func() genericModel {
		return genericModel{
			DockerImage:      types.StringValue("mysql:8"),
			DatabasePassword: types.StringValue("hunter2"),
			Env:              types.StringValue("A=1"),
			ExternalPort:     types.Int64Value(3306),
			Credentials: map[string]types.String{
				"database_root_password": types.StringValue("root1"),
				"database_name":          types.StringValue("app"),
			},
		}
	}

	state := base()
	plan := base()
	if deployNeeded(k, plan, state) {
		t.Error("identical models must not trigger a deploy")
	}

	plan = base()
	plan.Credentials = map[string]types.String{
		"database_root_password": types.StringValue("root2"),
		"database_name":          types.StringValue("app"),
	}
	if !deployNeeded(k, plan, state) {
		t.Error("a DeployTrigger credential attr change must trigger a deploy")
	}

	plan = base()
	plan.Credentials = map[string]types.String{
		"database_root_password": types.StringValue("root1"),
		"database_name":          types.StringValue("renamed"),
	}
	if deployNeeded(k, plan, state) {
		t.Error("a credential attr change with DeployTrigger unset must not trigger a deploy")
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

// mysqlLikeKind is a Kind shaped like MysqlKind's credential attrs, without
// depending on internal/client (kept here rather than reusing MysqlKind(nil)
// so these tests document exactly which CredentialAttr fields the bug and
// fix depend on: Computed, independent of DeployTrigger/RequiresReplace).
func mysqlLikeKind() Kind {
	return Kind{
		Name:      "mysql",
		HumanName: "MySQL",
		CredentialAttrs: []CredentialAttr{
			{TFName: "database_name", Required: true, RequiresReplace: true},
			{TFName: "database_user", Required: true, RequiresReplace: true},
			{TFName: "database_root_password", Computed: true},
		},
	}
}

// TestResolveCredentials_NullComputedCredentialUsesServerValue is the
// review-round-1 regression test for the destructive bug: a Computed
// credential attribute's PLANNED value can be a known null (not Unknown) —
// terraform-plugin-framework's UseStateForUnknown plan modifier copies a
// null prior state value forward as null verbatim ("Null is also a known
// value in Terraform and will be copied to the planned value by this plan
// modifier" — see resolveCredentials' doc comment for how state ends up
// null in the first place). Before the fix, resolveCredentials did not
// exist and Update built its request body with a bare
// `plan.Credentials[ca.TFName].ValueString()`, which returns "" for a null
// types.String exactly like it does for Unknown — and for
// database_root_password, "" is not a no-op: doc.go's dialect-C exception
// means the server CLEARS the stored root password on an explicit "".
//
// This test fails against that naive formula (asserted directly below via
// t.Run, so the regression is pinned in the same file rather than only in
// this test's history) and passes against resolveCredentials, which must
// substitute the server's current value instead.
func TestResolveCredentials_NullComputedCredentialUsesServerValue(t *testing.T) {
	k := mysqlLikeKind()
	plan := genericModel{
		Credentials: map[string]types.String{
			"database_name":          types.StringValue("app"),
			"database_user":          types.StringValue("app"),
			"database_root_password": types.StringNull(),
		},
	}
	current := &Object{
		Credentials: map[string]string{
			"database_root_password": "server-generated-secret",
		},
	}

	t.Run("naive ValueString() formula is the bug", func(t *testing.T) {
		got := plan.Credentials["database_root_password"].ValueString()
		if got != "" {
			t.Fatalf("test setup invalid: naive ValueString() = %q, want \"\" (the bug this test pins)", got)
		}
	})

	got := resolveCredentials(k, plan, current)
	if got["database_root_password"] != "server-generated-secret" {
		t.Errorf(`resolveCredentials()["database_root_password"] = %q, want "server-generated-secret" (a null planned value must fall back to the server's current value, never send "" and clear it)`, got["database_root_password"])
	}
}

// TestResolveCredentials_UnknownComputedCredentialUsesServerValue is the
// same defense on the Unknown half (the Create-time case setComputed
// already guarded — this test just confirms resolveCredentials treats it
// identically to Null, since ValueString() collapses both the same way).
func TestResolveCredentials_UnknownComputedCredentialUsesServerValue(t *testing.T) {
	k := mysqlLikeKind()
	plan := genericModel{
		Credentials: map[string]types.String{
			"database_name":          types.StringValue("app"),
			"database_user":          types.StringValue("app"),
			"database_root_password": types.StringUnknown(),
		},
	}
	current := &Object{
		Credentials: map[string]string{
			"database_root_password": "server-generated-secret",
		},
	}
	got := resolveCredentials(k, plan, current)
	if got["database_root_password"] != "server-generated-secret" {
		t.Errorf(`resolveCredentials()["database_root_password"] = %q, want "server-generated-secret"`, got["database_root_password"])
	}
}

// TestResolveCredentials_KnownValueSentVerbatim is the fix's other half: a
// genuinely known planned value (the user explicitly set
// database_root_password, or state legitimately carried forward a real
// value) must be sent as-is, not silently overridden by the server's
// current value — otherwise a deliberate change or clear (explicit "")
// could never reach the server at all.
func TestResolveCredentials_KnownValueSentVerbatim(t *testing.T) {
	k := mysqlLikeKind()
	for name, plannedValue := range map[string]string{
		"a new explicit value": "user-chosen-password",
		"an explicit clear":    "",
	} {
		t.Run(name, func(t *testing.T) {
			plan := genericModel{
				Credentials: map[string]types.String{
					"database_name":          types.StringValue("app"),
					"database_user":          types.StringValue("app"),
					"database_root_password": types.StringValue(plannedValue),
				},
			}
			current := &Object{
				Credentials: map[string]string{
					"database_root_password": "server-generated-secret",
				},
			}
			got := resolveCredentials(k, plan, current)
			if got["database_root_password"] != plannedValue {
				t.Errorf("resolveCredentials()[\"database_root_password\"] = %q, want %q (a known planned value must never be overridden)", got["database_root_password"], plannedValue)
			}
		})
	}
}

// TestResolveCredentials_NonComputedSentVerbatim pins that non-Computed
// (Required/RequiresReplace) credential attrs are never substituted —
// postgres's database_name/database_user never reach Update at all in
// practice (RequiresReplace), but resolveCredentials must not special-case
// them regardless.
func TestResolveCredentials_NonComputedSentVerbatim(t *testing.T) {
	k := mysqlLikeKind()
	plan := genericModel{
		Credentials: map[string]types.String{
			"database_name":          types.StringValue("mydb"),
			"database_user":          types.StringValue("myuser"),
			"database_root_password": types.StringValue("root1"),
		},
	}
	current := &Object{Credentials: map[string]string{
		"database_name":          "server-side-name",
		"database_user":          "server-side-user",
		"database_root_password": "server-generated-secret",
	}}
	got := resolveCredentials(k, plan, current)
	if got["database_name"] != "mydb" || got["database_user"] != "myuser" {
		t.Errorf("non-Computed credentials were substituted: %+v", got)
	}
}

// TestCredentialsNeedServerValue pins the carried fix from wave-2 task 6's
// review of task 5: resource.go's pre-Update Get must be skipped whenever
// resolveCredentials would never dereference it. credentialsNeedServerValue
// is the predicate that decides that — true iff some Computed
// CredentialAttr's planned value IsNull() or IsUnknown(), mirroring
// resolveCredentials' own per-attribute branch exactly, so the two can never
// disagree about whether `current` gets read.
func TestCredentialsNeedServerValue(t *testing.T) {
	k := mysqlLikeKind() // database_root_password Computed; database_name/user not

	cases := []struct {
		name string
		plan genericModel
		want bool
	}{
		{
			name: "Computed credential known -> no server read needed",
			plan: genericModel{Credentials: map[string]types.String{
				"database_name":          types.StringValue("app"),
				"database_user":          types.StringValue("app"),
				"database_root_password": types.StringValue("explicit-value"),
			}},
			want: false,
		},
		{
			name: "Computed credential null -> server read needed",
			plan: genericModel{Credentials: map[string]types.String{
				"database_name":          types.StringValue("app"),
				"database_user":          types.StringValue("app"),
				"database_root_password": types.StringNull(),
			}},
			want: true,
		},
		{
			name: "Computed credential unknown -> server read needed",
			plan: genericModel{Credentials: map[string]types.String{
				"database_name":          types.StringValue("app"),
				"database_user":          types.StringValue("app"),
				"database_root_password": types.StringUnknown(),
			}},
			want: true,
		},
		{
			name: "non-Computed credential null -> never needs a server read",
			plan: genericModel{Credentials: map[string]types.String{
				"database_name":          types.StringNull(),
				"database_user":          types.StringValue("app"),
				"database_root_password": types.StringValue("set"),
			}},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := credentialsNeedServerValue(k, tc.plan); got != tc.want {
				t.Errorf("credentialsNeedServerValue() = %v, want %v", got, tc.want)
			}
		})
	}

	// Zero-CredentialAttrs Kind (redis/mongo/postgres-shaped): must always be
	// false regardless of what Credentials happens to hold, since there is no
	// CredentialAttr to iterate at all.
	zeroKind := Kind{Name: "redis"}
	anyPlan := genericModel{Credentials: map[string]types.String{"leftover": types.StringNull()}}
	if credentialsNeedServerValue(zeroKind, anyPlan) {
		t.Error("a zero-CredentialAttr Kind must never need a server read")
	}
}

// TestResolveUnknownComputedCredentials_ResolvesViaGet pins
// persistPartial's matching defense: an Unknown Computed credential must be
// resolved from the server (via a best-effort Get) rather than committed
// Unknown — Terraform core normalizes a remaining Unknown in an errored
// Create's persisted state to null, which is exactly the shape
// resolveCredentials' tests above prove is dangerous on the next Update.
func TestResolveUnknownComputedCredentials_ResolvesViaGet(t *testing.T) {
	k := mysqlLikeKind()
	k.Client.Get = func(_ context.Context, id string) (*Object, error) {
		if id != "mysql-1" {
			t.Errorf("Get called with id %q, want mysql-1", id)
		}
		return &Object{Credentials: map[string]string{"database_root_password": "server-generated-secret"}}, nil
	}
	m := genericModel{
		ID: types.StringValue("mysql-1"),
		Credentials: map[string]types.String{
			"database_name":          types.StringValue("app"),
			"database_user":          types.StringValue("app"),
			"database_root_password": types.StringUnknown(),
		},
	}
	resolveUnknownComputedCredentials(context.Background(), k, "mysql-1", &m)
	got := m.Credentials["database_root_password"]
	if got.IsUnknown() {
		t.Fatal("database_root_password left Unknown: Terraform core will normalize this to null in errored-apply state")
	}
	if got.ValueString() != "server-generated-secret" {
		t.Errorf("database_root_password = %q, want server-generated-secret", got.ValueString())
	}
}

// TestResolveUnknownComputedCredentials_NoOpWhenAlreadyKnown pins that this
// is a targeted defense, not a blanket refresh: it must not call Get (and
// must not touch Credentials) when no Computed credential is Unknown.
func TestResolveUnknownComputedCredentials_NoOpWhenAlreadyKnown(t *testing.T) {
	k := mysqlLikeKind()
	called := false
	k.Client.Get = func(_ context.Context, _ string) (*Object, error) {
		called = true
		return &Object{}, nil
	}
	m := genericModel{
		Credentials: map[string]types.String{
			"database_root_password": types.StringValue("already-known"),
		},
	}
	resolveUnknownComputedCredentials(context.Background(), k, "mysql-1", &m)
	if called {
		t.Error("Get was called even though no Computed credential was Unknown")
	}
	if got := m.Credentials["database_root_password"].ValueString(); got != "already-known" {
		t.Errorf("database_root_password = %q, want unchanged already-known", got)
	}
}

// TestResolveUnknownComputedCredentials_BestEffortIgnoresGetError pins the
// documented best-effort behavior: if the resolving Get itself fails, the
// Unknown value is left as-is (there is nothing better to do — the caller,
// persistPartial, already adds its own diagnostic regardless).
func TestResolveUnknownComputedCredentials_BestEffortIgnoresGetError(t *testing.T) {
	k := mysqlLikeKind()
	k.Client.Get = func(_ context.Context, _ string) (*Object, error) {
		return nil, fmt.Errorf("boom")
	}
	m := genericModel{
		Credentials: map[string]types.String{
			"database_root_password": types.StringUnknown(),
		},
	}
	resolveUnknownComputedCredentials(context.Background(), k, "mysql-1", &m)
	if !m.Credentials["database_root_password"].IsUnknown() {
		t.Errorf("database_root_password = %v, want left Unknown when the resolving Get fails", m.Credentials["database_root_password"])
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
