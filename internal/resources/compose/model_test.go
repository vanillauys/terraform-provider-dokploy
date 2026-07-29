package compose

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func strPtr(s string) *string { return &s }

// doc.go records that composeFile, composePath, command and suffix read back
// as a literal "" rather than null - composeFile does so on a freshly
// API-created record, which makes compose the one place in this provider
// where the "" form is the SERVER's own default rather than a UI artefact.
// Every one must collapse to a null attribute or the resource never
// converges. The structural half is TestNoStringPointerValueOutsideExemptions
// in internal/tfutil.
func TestFlattenEmptyStringsBecomeNull(t *testing.T) {
	c := &client.Compose{
		ComposeID: "c1", Name: "web", EnvironmentID: "env1",
		SourceType: "github", ComposeType: "docker-compose",
		ComposeFile: "", ComposePath: "", Command: "", Suffix: "",
		Description: strPtr(""), Env: strPtr(""), ServerID: strPtr(""),
	}

	var m resourceModel
	if diags := flatten(context.Background(), c, &m); diags.HasError() {
		t.Fatalf("flatten: %v", diags)
	}

	for name, got := range map[string]types.String{
		"compose_path": m.ComposePath,
		"command":      m.Command,
		"suffix":       m.Suffix,
		"description":  m.Description,
		"env":          m.Env,
		"server_id":    m.ServerID,
	} {
		if !got.IsNull() {
			t.Errorf("%s = %q, want null: a \"\" from the server must collapse to null", name, got.ValueString())
		}
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
	for name, got := range map[string]types.Bool{
		"enable_submodules":           m.EnableSubmodules,
		"randomize":                   m.Randomize,
		"isolated_deployment":         m.IsolatedDeployment,
		"isolated_deployments_volume": m.IsolatedDeploymentsVolume,
	} {
		if !got.IsNull() {
			t.Errorf("%s = %v, want null", name, got)
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
		"composePath": req.ComposePath,
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
