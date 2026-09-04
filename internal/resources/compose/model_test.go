package compose

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func strPtr(s string) *string { return &s }

// doc.go records that composeFile, command and suffix read back as a literal
// "" rather than null - composeFile does so on a freshly API-created record,
// which makes compose the one place in this provider where the "" form is the
// SERVER's own default rather than a UI artefact. Each must collapse to a
// null attribute or the resource never converges. The structural half is
// TestNoStringPointerValueOutsideExemptions in internal/tfutil.
//
// compose_path is deliberately absent from this list; see
// TestFlattenKeepsComposePathVerbatim.
func TestFlattenEmptyStringsBecomeNull(t *testing.T) {
	c := &client.Compose{
		ComposeID: "c1", Name: "web", EnvironmentID: "env1",
		SourceType: "github", ComposeType: "docker-compose",
		ComposeFile: "", ComposePath: "./docker-compose.yml", Command: "", Suffix: "",
		Description: strPtr(""), Env: strPtr(""), ServerID: strPtr(""),
	}

	var m resourceModel
	if diags := flatten(context.Background(), c, &m); diags.HasError() {
		t.Fatalf("flatten: %v", diags)
	}

	for name, got := range map[string]types.String{
		"command":     m.Command,
		"suffix":      m.Suffix,
		"description": m.Description,
		"env":         m.Env,
		"server_id":   m.ServerID,
	} {
		if !got.IsNull() {
			t.Errorf("%s = %q, want null: a \"\" from the server must collapse to null", name, got.ValueString())
		}
	}
}

// compose_path is the one string here that must NOT collapse. compose.update
// gives it a minimum length of 1, so "" is a 400 and the server always holds
// a real path - defaulting to ./docker-compose.yml. The attribute is
// Optional+Computed with that same default, so collapsing a server value to
// null would make the very common default-valued record diff forever against
// its own default.
func TestFlattenKeepsComposePathVerbatim(t *testing.T) {
	c := &client.Compose{ComposeID: "c1", SourceType: "raw", ComposePath: "./docker-compose.yml"}

	var m resourceModel
	if diags := flatten(context.Background(), c, &m); diags.HasError() {
		t.Fatalf("flatten: %v", diags)
	}
	if m.ComposePath.IsNull() {
		t.Fatal("compose_path collapsed to null; it is Optional+Computed with a default and the server never stores \"\"")
	}
	if got := m.ComposePath.ValueString(); got != "./docker-compose.yml" {
		t.Errorf("compose_path = %q", got)
	}
}

// The raw source's compose_file is the same "" hazard one level down, inside
// a nested block, where the guard's AST walk still sees the call but a
// reviewer might not.
func TestFlattenEmptyComposeFileBecomesNull(t *testing.T) {
	c := &client.Compose{ComposeID: "c1", SourceType: "raw", ComposeFile: ""}

	var m resourceModel
	if diags := flatten(context.Background(), c, &m); diags.HasError() {
		t.Fatalf("flatten: %v", diags)
	}
	if m.Raw == nil {
		t.Fatal("raw block is nil on a raw-sourced record")
	}
	if !m.Raw.ComposeFile.IsNull() {
		t.Errorf("raw.compose_file = %q, want null", m.Raw.ComposeFile.ValueString())
	}
}

// A null triggerType/autoDeploy is a real stored state. Mapping it to false
// would make Terraform report a value the server does not hold.
func TestFlattenNullableColumnsStayNull(t *testing.T) {
	c := &client.Compose{ComposeID: "c1", SourceType: "github"}

	var m resourceModel
	if diags := flatten(context.Background(), c, &m); diags.HasError() {
		t.Fatalf("flatten: %v", diags)
	}

	if !m.AutoDeploy.IsNull() {
		t.Errorf("auto_deploy = %v, want null: the column is nullable, not defaulted", m.AutoDeploy)
	}
	if !m.TriggerType.IsNull() {
		t.Errorf("trigger_type = %v, want null", m.TriggerType)
	}
}

// The other two booleans are NOT NULL server-side: an explicit null is
// coerced to false on write. They must resolve to a concrete false rather
// than stay null, or they diff forever against their own schema default.
func TestFlattenResolvesNotNullBooleansToFalse(t *testing.T) {
	c := &client.Compose{ComposeID: "c1", SourceType: "github"}

	var m resourceModel
	if diags := flatten(context.Background(), c, &m); diags.HasError() {
		t.Fatalf("flatten: %v", diags)
	}

	for name, got := range map[string]types.Bool{
		"enable_submodules": m.EnableSubmodules,
		"randomize":         m.Randomize,
	} {
		if got.IsNull() || got.IsUnknown() {
			t.Errorf("%s = %v, want a concrete bool: the column is NOT NULL server-side", name, got)
		}
		if got.ValueBool() {
			t.Errorf("%s = true, want false", name)
		}
	}
}

// The source block is chosen by the server's discriminator, never by which
// columns are non-null. A record retargeted from git to github keeps its
// stale customGitUrl - the same corrupt shape mount, backup and schedule can
// all reach - so reading the columns directly would populate two blocks.
func TestFlattenPicksTheSourceBlockNamedBySourceType(t *testing.T) {
	c := &client.Compose{
		ComposeID: "c1", SourceType: "github",
		Repository: strPtr("site"), Owner: strPtr("acme"), Branch: strPtr("main"),
		GithubID: strPtr("gh1"),
		// Stale, left behind by an earlier git source.
		CustomGitURL: strPtr("git@example.com:old/repo.git"),
	}

	var m resourceModel
	if diags := flatten(context.Background(), c, &m); diags.HasError() {
		t.Fatalf("flatten: %v", diags)
	}

	if m.Github == nil {
		t.Fatal("github block is nil on a github-sourced record")
	}
	if m.Git != nil {
		t.Errorf("git block populated from a stale customGitUrl; the discriminator says github")
	}
	if m.Raw != nil {
		t.Errorf("raw block populated on a github-sourced record")
	}
	if m.Github.Repository.ValueString() != "site" {
		t.Errorf("repository = %q", m.Github.Repository.ValueString())
	}
}

func TestSourceTypeFor(t *testing.T) {
	for _, tc := range []struct {
		name string
		m    resourceModel
		want string
	}{
		{"github", resourceModel{Github: &githubSource{}}, "github"},
		{"git", resourceModel{Git: &gitSource{}}, "git"},
		{"raw", resourceModel{Raw: &rawSource{}}, "raw"},
		{"none defaults to github", resourceModel{}, "github"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sourceTypeFor(&tc.m); got != tc.want {
				t.Errorf("sourceTypeFor = %q, want %q", got, tc.want)
			}
		})
	}
}

// Switching source mode must CLEAR the previous mode's columns, not leave
// them stale. Every source column is transmitted on every update, so the
// unused mode's fields marshal to explicit nulls.
func TestExpandUpdateClearsTheUnusedSourceColumns(t *testing.T) {
	m := resourceModel{
		ID: types.StringValue("c1"), Name: types.StringValue("web"),
		Git: &gitSource{URL: types.StringValue("git@example.com:acme/site.git")},
	}

	req, diags := expandUpdate(context.Background(), &m)
	if diags.HasError() {
		t.Fatalf("expandUpdate: %v", diags)
	}

	if req.SourceType != "git" {
		t.Errorf("sourceType = %q, want git", req.SourceType)
	}
	for name, got := range map[string]*string{
		"repository": req.Repository,
		"owner":      req.Owner,
		"branch":     req.Branch,
		"githubId":   req.GithubID,
	} {
		if got != nil {
			t.Errorf("%s = %q, want nil so the github columns are cleared on a switch to git", name, *got)
		}
	}
	if req.ComposeFile != "" {
		t.Errorf("composeFile = %q, want \"\" so a previous raw source is cleared", req.ComposeFile)
	}
}

// The dialect C fields must reach the wire as "" when the attribute is null.
// Sending nil would marshal to an explicit null, which is a 400 on these
// five.
func TestExpandUpdateMapsNullToEmptyStringOnDialectCFields(t *testing.T) {
	m := resourceModel{ID: types.StringValue("c1")}

	req, diags := expandUpdate(context.Background(), &m)
	if diags.HasError() {
		t.Fatalf("expandUpdate: %v", diags)
	}

	for name, got := range map[string]string{
		"name":        req.Name,
		"command":     req.Command,
		"suffix":      req.Suffix,
		"composeFile": req.ComposeFile,
	} {
		if got != "" {
			t.Errorf("%s = %q, want \"\"", name, got)
		}
	}
}

// compose.create accepts only seven fields. Anything else set here would be
// silently dropped by the server, so the follow-up update in resource.go is
// what actually applies the source block - this pins that expandCreate does
// not pretend otherwise.
func TestExpandCreateCarriesOnlyTheSevenAcceptedFields(t *testing.T) {
	m := resourceModel{
		Name:          types.StringValue("web"),
		EnvironmentID: types.StringValue("env1"),
		ComposeType:   types.StringValue("stack"),
		Github:        &githubSource{Repository: types.StringValue("site")},
		Command:       types.StringValue("echo hi"),
	}

	req := expandCreate(&m)

	if req.Name != "web" || req.EnvironmentID != "env1" || req.ComposeType != "stack" {
		t.Errorf("create request = %+v", req)
	}
	// The raw source is the only one whose payload create can carry.
	if req.ComposeFile != "" {
		t.Errorf("composeFile = %q, want \"\" for a github source", req.ComposeFile)
	}
}

func TestExpandCreateCarriesTheRawComposeFile(t *testing.T) {
	m := resourceModel{
		Name:          types.StringValue("web"),
		EnvironmentID: types.StringValue("env1"),
		Raw:           &rawSource{ComposeFile: types.StringValue("services: {}")},
	}

	if got := expandCreate(&m).ComposeFile; got != "services: {}" {
		t.Errorf("composeFile = %q", got)
	}
}

// v0.30.0: createEnvFile is a bare server bool - no null case to defend
// against here, unlike the four booleans in
// TestFlattenResolvesNotNullBooleansToFalse. icon follows the same
// ""/null-collapse rule as the other optional strings.
func TestFlattenCreateEnvFileAndIcon(t *testing.T) {
	c := &client.Compose{
		ComposeID: "c1", SourceType: "raw",
		CreateEnvFile: true, Icon: strPtr("lucide:cloud"),
	}

	var m resourceModel
	if diags := flatten(context.Background(), c, &m); diags.HasError() {
		t.Fatalf("flatten: %v", diags)
	}
	if !m.CreateEnvFile.ValueBool() {
		t.Error("create_env_file = false, want true")
	}
	if got := m.Icon.ValueString(); got != "lucide:cloud" {
		t.Errorf("icon = %q, want lucide:cloud", got)
	}
}

func TestFlattenIconEmptyStringBecomesNull(t *testing.T) {
	c := &client.Compose{ComposeID: "c1", SourceType: "raw", Icon: strPtr("")}

	var m resourceModel
	if diags := flatten(context.Background(), c, &m); diags.HasError() {
		t.Fatalf("flatten: %v", diags)
	}
	if !m.Icon.IsNull() {
		t.Errorf("icon = %q, want null", m.Icon.ValueString())
	}
}

// serviceNetworks follows the same null-vs-[] collapse as tfutil's
// StringSetOrNull: doc.go records that a fresh create returns [] and an
// explicit clear reads back a literal null - neither shape may survive as a
// distinct Terraform value, or the two would diff against each other
// forever.
func TestFlattenServiceNetworksNilBecomesNullSet(t *testing.T) {
	c := &client.Compose{ComposeID: "c1", SourceType: "raw", ServiceNetworks: nil}

	var m resourceModel
	if diags := flatten(context.Background(), c, &m); diags.HasError() {
		t.Fatalf("flatten: %v", diags)
	}
	if !m.ServiceNetworks.IsNull() {
		t.Errorf("service_networks = %v, want null", m.ServiceNetworks)
	}
}

func TestFlattenServiceNetworksEmptyBecomesNullSet(t *testing.T) {
	c := &client.Compose{ComposeID: "c1", SourceType: "raw", ServiceNetworks: []client.ComposeServiceNetwork{}}

	var m resourceModel
	if diags := flatten(context.Background(), c, &m); diags.HasError() {
		t.Fatalf("flatten: %v", diags)
	}
	if !m.ServiceNetworks.IsNull() {
		t.Errorf("service_networks = %v, want null", m.ServiceNetworks)
	}
}

func TestFlattenServiceNetworksOneEntryRoundTrips(t *testing.T) {
	c := &client.Compose{
		ComposeID: "c1", SourceType: "raw",
		ServiceNetworks: []client.ComposeServiceNetwork{
			{ServiceName: "web", NetworkIDs: []string{"net1"}, DetachDokployNetwork: true},
		},
	}

	var m resourceModel
	if diags := flatten(context.Background(), c, &m); diags.HasError() {
		t.Fatalf("flatten: %v", diags)
	}
	if m.ServiceNetworks.IsNull() {
		t.Fatal("service_networks = null, want one entry")
	}

	var models []serviceNetworkModel
	if diags := m.ServiceNetworks.ElementsAs(context.Background(), &models, false); diags.HasError() {
		t.Fatalf("reading back service_networks: %v", diags)
	}
	if len(models) != 1 {
		t.Fatalf("service_networks has %d entries, want 1", len(models))
	}
	if models[0].ServiceName.ValueString() != "web" {
		t.Errorf("service_name = %q, want web", models[0].ServiceName.ValueString())
	}
	if models[0].DetachDokployNetwork.ValueBool() != true {
		t.Error("detach_dokploy_network = false, want true")
	}

	var ids []string
	if diags := models[0].NetworkIDs.ElementsAs(context.Background(), &ids, false); diags.HasError() {
		t.Fatalf("reading back network_ids: %v", diags)
	}
	if len(ids) != 1 || ids[0] != "net1" {
		t.Errorf("network_ids = %v, want [net1]", ids)
	}
}

// expandUpdate's inverse: a null/unknown set means "no change intended by
// this attribute", which maps to an explicit JSON null on the wire - dialect
// B, matching icon and every other nullable field in this group.
func TestExpandUpdateServiceNetworksAndIcon(t *testing.T) {
	m := resourceModel{
		ID:              types.StringValue("c1"),
		Icon:            types.StringNull(),
		ServiceNetworks: types.SetNull(types.ObjectType{AttrTypes: serviceNetworkAttrTypes}),
	}

	req, diags := expandUpdate(context.Background(), &m)
	if diags.HasError() {
		t.Fatalf("expandUpdate: %v", diags)
	}
	if req.Icon != nil {
		t.Errorf("icon = %q, want nil", *req.Icon)
	}
	if req.ServiceNetworks != nil {
		t.Errorf("serviceNetworks = %v, want nil (explicit null on the wire)", *req.ServiceNetworks)
	}
}

func TestExpandUpdateServiceNetworksOneEntry(t *testing.T) {
	ctx := context.Background()
	idsSet, diags := types.SetValueFrom(ctx, types.StringType, []string{"net1"})
	if diags.HasError() {
		t.Fatalf("building network_ids set: %v", diags)
	}
	sn := serviceNetworkModel{
		ServiceName:          types.StringValue("web"),
		NetworkIDs:           idsSet,
		DetachDokployNetwork: types.BoolValue(true),
	}
	set, diags := types.SetValueFrom(ctx, types.ObjectType{AttrTypes: serviceNetworkAttrTypes}, []serviceNetworkModel{sn})
	if diags.HasError() {
		t.Fatalf("building service_networks set: %v", diags)
	}

	m := resourceModel{ID: types.StringValue("c1"), ServiceNetworks: set}

	req, expDiags := expandUpdate(ctx, &m)
	if expDiags.HasError() {
		t.Fatalf("expandUpdate: %v", expDiags)
	}
	if req.ServiceNetworks == nil {
		t.Fatal("serviceNetworks = nil, want one entry")
	}
	got := *req.ServiceNetworks
	if len(got) != 1 {
		t.Fatalf("serviceNetworks has %d entries, want 1", len(got))
	}
	if got[0].ServiceName != "web" {
		t.Errorf("serviceName = %q, want web", got[0].ServiceName)
	}
	if !got[0].DetachDokployNetwork {
		t.Error("detachDokployNetwork = false, want true")
	}
	if len(got[0].NetworkIDs) != 1 || got[0].NetworkIDs[0] != "net1" {
		t.Errorf("networkIds = %v, want [net1]", got[0].NetworkIDs)
	}
}
