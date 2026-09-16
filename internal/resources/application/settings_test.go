package application

import (
	"context"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func previewObject(t *testing.T, p previewModel) types.Object {
	t.Helper()
	obj, d := types.ObjectValueFrom(context.Background(), previewAttrTypes, p)
	if d.HasError() {
		t.Fatalf("preview object: %v", d)
	}
	return obj
}

func fullPreview(secret, wo types.String) previewModel {
	return previewModel{
		Enabled:               types.BoolValue(true),
		Env:                   types.StringValue("A=1"),
		BuildArgs:             types.StringValue("B=2"),
		BuildSecrets:          secret,
		BuildSecretsWo:        wo,
		BuildSecretsWoVersion: types.Int64Value(1),
		CertificateType:       types.StringValue("letsencrypt"),
		CustomCertResolver:    types.StringValue("resolver"),
		HTTPS:                 types.BoolValue(true),
		Labels:                types.ListValueMust(types.StringType, []attr.Value{types.StringValue("a=b")}),
		Limit:                 types.Int64Value(5),
		Path:                  types.StringValue("/p"),
		Port:                  types.Int64Value(4321),
		// True, so that the zero-value sweep in
		// TestUpdateRequestReadsEveryFieldFromTheModel sees a set value.
		RequireCollaboratorPermissions: types.BoolValue(true),
		Wildcard:                       types.StringValue("*.p.example.com"),
	}
}

// A null block writes the server defaults of a fresh record: application.update
// is dialect B, so anything else would keep a value from the Dokploy UI.
func TestPreviewRequestNullBlockWritesDefaults(t *testing.T) {
	var diags diag.Diagnostics
	got := previewRequest(context.Background(), types.ObjectNull(previewAttrTypes), types.ObjectNull(previewAttrTypes), &diags)
	if diags.HasError() {
		t.Fatal(diags)
	}
	want := client.ApplicationPreviewUpdate{
		PreviewCertificateType: "none", PreviewLimit: 3, PreviewPath: "/", PreviewPort: 3000,
		PreviewRequireCollaboratorPermissions: true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("previewRequest(null) = %+v, want %+v", got, want)
	}
	if got.PreviewLabels != nil || got.PreviewEnv != nil || got.PreviewBuildSecrets != nil {
		t.Errorf("null block must send null for the pointer fields, got %+v", got)
	}
}

// Every field of the block reaches the request; the secret comes from the
// plain attribute, or from the config's write-only form when the plain one
// is null, and is null when neither is set.
func TestPreviewRequestReadsEveryField(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	plan := previewObject(t, fullPreview(types.StringValue("S=3"), types.StringNull()))
	got := previewRequest(ctx, plan, plan, &diags)
	if diags.HasError() {
		t.Fatal(diags)
	}
	v := reflect.ValueOf(got)
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() == reflect.Pointer && f.IsNil() {
			t.Errorf("field %s is nil despite a fully populated block", v.Type().Field(i).Name)
		}
	}
	if got.PreviewBuildSecrets == nil || *got.PreviewBuildSecrets != "S=3" {
		t.Errorf("plain secret = %v, want S=3", got.PreviewBuildSecrets)
	}
	if got.PreviewLabels == nil || len(*got.PreviewLabels) != 1 || (*got.PreviewLabels)[0] != "a=b" {
		t.Errorf("labels = %v", got.PreviewLabels)
	}

	// Write-only: the plan holds null for build_secrets, the config holds
	// the value.
	planWo := previewObject(t, fullPreview(types.StringNull(), types.StringNull()))
	cfgWo := previewObject(t, fullPreview(types.StringNull(), types.StringValue("W=4")))
	got = previewRequest(ctx, planWo, cfgWo, &diags)
	if got.PreviewBuildSecrets == nil || *got.PreviewBuildSecrets != "W=4" {
		t.Errorf("write-only secret = %v, want W=4", got.PreviewBuildSecrets)
	}

	// Neither: null on the wire clears the column.
	got = previewRequest(ctx, planWo, planWo, &diags)
	if got.PreviewBuildSecrets != nil {
		t.Errorf("no secret = %q, want nil", *got.PreviewBuildSecrets)
	}
}

func defaultApp() *client.Application {
	return &client.Application{ApplicationPreview: client.ApplicationPreview{
		PreviewCertificateType: "none", PreviewLimit: 3, PreviewPath: "/", PreviewPort: 3000,
		PreviewRequireCollaboratorPermissions: true,
	}}
}

// The null-or-block decision on Read follows the prior state: a null prior
// stays null while the server holds the defaults, a non-null prior stays a
// block, and drift from the defaults is always a block.
func TestFlattenPreviewFollowsPriorShape(t *testing.T) {
	ctx := context.Background()
	empty := ""
	drifted := defaultApp()
	drifted.PreviewLimit = 5

	cases := []struct {
		name     string
		app      *client.Application
		prior    types.Object
		wantNull bool
	}{
		{"defaults, null prior", defaultApp(), types.ObjectNull(previewAttrTypes), true},
		{"defaults with empty strings, null prior", func() *client.Application {
			a := defaultApp()
			a.PreviewEnv, a.PreviewWildcard, a.PreviewLabels = &empty, &empty, []string{}
			return a
		}(), types.ObjectNull(previewAttrTypes), true},
		{"defaults, block prior", defaultApp(), previewObject(t, previewModel{Limit: types.Int64Value(3), Labels: types.ListNull(types.StringType)}), false},
		{"drift, null prior", drifted, types.ObjectNull(previewAttrTypes), false},
	}
	for _, tc := range cases {
		var diags diag.Diagnostics
		got := flattenPreview(ctx, tc.app, tc.prior, &diags)
		if diags.HasError() {
			t.Fatalf("%s: %v", tc.name, diags)
		}
		if got.IsNull() != tc.wantNull {
			t.Errorf("%s: null = %v, want %v", tc.name, got.IsNull(), tc.wantNull)
		}
	}
}

// The version companion is not on the server; Read carries it from the
// prior state. The write-only form itself is always null in state.
func TestFlattenPreviewCarriesVersionAndHidesSecret(t *testing.T) {
	ctx := context.Background()
	app := defaultApp()
	secret := "S=3"
	app.PreviewBuildSecrets = &secret
	app.PreviewLimit = 5
	// A hand-built block must state the list's element type: a zero-value
	// types.List has none and fails the object conversion.
	prior := previewObject(t, previewModel{BuildSecretsWoVersion: types.Int64Value(7), Labels: types.ListNull(types.StringType)})

	var diags diag.Diagnostics
	m := resourceModel{PreviewDeployments: flattenPreview(ctx, app, prior, &diags)}
	var p previewModel
	decodeObject(ctx, m.PreviewDeployments, &p, &diags)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if p.BuildSecretsWoVersion.ValueInt64() != 7 || !p.BuildSecretsWo.IsNull() || p.BuildSecrets.ValueString() != "S=3" {
		t.Errorf("flattened block = %+v", p)
	}

	hideWriteOnly(ctx, &m, map[string]bool{previewSecretName: true}, &diags)
	decodeObject(ctx, m.PreviewDeployments, &p, &diags)
	if !p.BuildSecrets.IsNull() || p.Limit.ValueInt64() != 5 {
		t.Errorf("after hideWriteOnly = %+v, want build_secrets null and the rest kept", p)
	}
}

func TestFlattenRollbackFollowsPriorShape(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	reg := "reg-1"
	if got := flattenRollback(ctx, &client.Application{}, types.ObjectNull(rollbackAttrTypes), &diags); !got.IsNull() {
		t.Error("defaults with a null prior must stay null")
	}
	if got := flattenRollback(ctx, &client.Application{}, types.ObjectValueMust(rollbackAttrTypes, map[string]attr.Value{
		"enabled": types.BoolValue(false), "registry_id": types.StringNull(),
	}), &diags); got.IsNull() {
		t.Error("defaults with a block prior must stay a block")
	}
	app := &client.Application{ApplicationRollback: client.ApplicationRollback{RollbackActive: true, RollbackRegistryID: &reg}}
	got := flattenRollback(ctx, app, types.ObjectNull(rollbackAttrTypes), &diags)
	var r rollbackModel
	decodeObject(ctx, got, &r, &diags)
	if diags.HasError() || !r.Enabled.ValueBool() || r.RegistryID.ValueString() != "reg-1" {
		t.Errorf("drift = %+v (%v)", r, diags)
	}
}

// updateRequest is dialect B: a field that no attribute feeds is a field
// the resource can never change. Every non-embedded field and every field
// of the three embedded blocks must come out non-zero from a fully
// populated model.
func TestUpdateRequestReadsEveryFieldFromTheModel(t *testing.T) {
	ctx := context.Background()
	preview := previewObject(t, fullPreview(types.StringValue("S=3"), types.StringNull()))
	m := resourceModel{
		ID:                   types.StringValue("app1"),
		Name:                 types.StringValue("web"),
		Description:          types.StringValue("desc"),
		AutoDeploy:           types.BoolValue(true),
		Replicas:             types.Int64Value(2),
		CPULimit:             types.StringValue("1"),
		MemoryLimit:          types.StringValue("1"),
		CPUReservation:       types.StringValue("1"),
		MemoryReservation:    types.StringValue("1"),
		Command:              types.StringValue("/bin/run"),
		Args:                 types.ListValueMust(types.StringType, []attr.Value{types.StringValue("-x")}),
		RegistryID:           types.StringValue("reg"),
		NetworkIDs:           types.SetValueMust(types.StringType, []attr.Value{types.StringValue("net")}),
		DetachDokployNetwork: types.BoolValue(true),
		Title:                types.StringValue("Web"),
		Subtitle:             types.StringValue("Site"),
		PreviewDeployments:   preview,
		Rollback: types.ObjectValueMust(rollbackAttrTypes, map[string]attr.Value{
			"enabled": types.BoolValue(true), "registry_id": types.StringValue("reg-2"),
		}),
		BuildServerID:   types.StringValue("srv"),
		BuildRegistryID: types.StringValue("reg-3"),
		CleanCache:      types.BoolValue(true),
		DropBuildPath:   types.StringValue("/drop"),
	}
	req, d := updateRequest(ctx, "app1", m, m)
	if d.HasError() {
		t.Fatal(d)
	}
	var check func(v reflect.Value, prefix string)
	check = func(v reflect.Value, prefix string) {
		for i := 0; i < v.NumField(); i++ {
			f, ft := v.Field(i), v.Type().Field(i)
			if ft.Anonymous {
				check(f, ft.Name+".")
				continue
			}
			zero := f.IsZero()
			if f.Kind() == reflect.Pointer {
				zero = f.IsNil()
			}
			if zero {
				t.Errorf("%s%s is unset despite a fully populated model; the resource can never write it", prefix, ft.Name)
			}
		}
	}
	check(reflect.ValueOf(req), "")
}
