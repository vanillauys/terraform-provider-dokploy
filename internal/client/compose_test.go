package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The fixture is a real compose.one body from the rig (v0.29.13,
// 2026-07-29), trimmed to the fields this client models.
const composeOneFixture = `{
  "composeId":"c1","name":"web","appName":"web-abc123","description":"the site",
  "environmentId":"env1","composeStatus":"idle","composeType":"docker-compose",
  "sourceType":"github","composeFile":"","composePath":"./docker-compose.yml",
  "command":"","suffix":"","repository":"site","owner":"acme","branch":"main",
  "githubId":"gh1","customGitUrl":null,"customGitBranch":null,
  "customGitSSHKeyId":null,"triggerType":"push","autoDeploy":true,
  "enableSubmodules":false,"randomize":false,"isolatedDeployment":false,
  "isolatedDeploymentsVolume":false,"watchPaths":["src/**"],"env":"K=v",
  "serverId":null,"createdAt":"2026-07-29T06:40:44.191Z",
  "domains":[],"mounts":[]
}`

// Every field is asserted deliberately: a tag typo on an unasserted field
// decodes silently wrong and stays green.
func TestComposeDecode(t *testing.T) {
	var c Compose
	if err := json.Unmarshal([]byte(composeOneFixture), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if c.ComposeID != "c1" {
		t.Errorf("ComposeID = %q", c.ComposeID)
	}
	if c.Name != "web" {
		t.Errorf("Name = %q", c.Name)
	}
	if c.AppName != "web-abc123" {
		t.Errorf("AppName = %q", c.AppName)
	}
	if c.Description == nil || *c.Description != "the site" {
		t.Errorf("Description = %v", c.Description)
	}
	if c.EnvironmentID != "env1" {
		t.Errorf("EnvironmentID = %q", c.EnvironmentID)
	}
	if c.ComposeStatus != "idle" {
		t.Errorf("ComposeStatus = %q", c.ComposeStatus)
	}
	if c.ComposeType != "docker-compose" {
		t.Errorf("ComposeType = %q", c.ComposeType)
	}
	if c.SourceType != "github" {
		t.Errorf("SourceType = %q", c.SourceType)
	}
	if c.ComposeFile != "" {
		t.Errorf("ComposeFile = %q, want the empty string a non-raw source stores", c.ComposeFile)
	}
	if c.ComposePath != "./docker-compose.yml" {
		t.Errorf("ComposePath = %q", c.ComposePath)
	}
	if c.Command != "" {
		t.Errorf("Command = %q", c.Command)
	}
	if c.Suffix != "" {
		t.Errorf("Suffix = %q", c.Suffix)
	}
	if c.Repository == nil || *c.Repository != "site" {
		t.Errorf("Repository = %v", c.Repository)
	}
	if c.Owner == nil || *c.Owner != "acme" {
		t.Errorf("Owner = %v", c.Owner)
	}
	if c.Branch == nil || *c.Branch != "main" {
		t.Errorf("Branch = %v", c.Branch)
	}
	if c.GithubID == nil || *c.GithubID != "gh1" {
		t.Errorf("GithubID = %v", c.GithubID)
	}
	if c.CustomGitURL != nil || c.CustomGitBranch != nil || c.CustomGitSSHKeyID != nil {
		t.Errorf("custom git fields should be nil on a github-sourced record")
	}
	if c.TriggerType == nil || *c.TriggerType != "push" {
		t.Errorf("TriggerType = %v", c.TriggerType)
	}
	if c.AutoDeploy == nil || !*c.AutoDeploy {
		t.Errorf("AutoDeploy = %v", c.AutoDeploy)
	}
	for name, got := range map[string]*bool{
		"EnableSubmodules":          c.EnableSubmodules,
		"Randomize":                 c.Randomize,
		"IsolatedDeployment":        c.IsolatedDeployment,
		"IsolatedDeploymentsVolume": c.IsolatedDeploymentsVolume,
	} {
		if got == nil || *got {
			t.Errorf("%s = %v, want a non-nil false", name, got)
		}
	}
	if len(c.WatchPaths) != 1 || c.WatchPaths[0] != "src/**" {
		t.Errorf("WatchPaths = %v", c.WatchPaths)
	}
	if c.Env == nil || *c.Env != "K=v" {
		t.Errorf("Env = %v", c.Env)
	}
	if c.ServerID != nil {
		t.Errorf("ServerID = %v", c.ServerID)
	}
	if c.CreatedAt != "2026-07-29T06:40:44.191Z" {
		t.Errorf("CreatedAt = %q", c.CreatedAt)
	}
	if c.Domains == nil || c.Mounts == nil {
		t.Errorf("embedded collections should decode to empty slices, not nil")
	}
}

// A null triggerType/autoDeploy is a real stored state, not a decode edge
// case: doc.go records that an explicit null on either column is accepted
// and stored. Bare bool/string fields would read both back as false/"".
func TestComposeDecodeNullableColumns(t *testing.T) {
	var c Compose
	if err := json.Unmarshal([]byte(`{"composeId":"c1","triggerType":null,"autoDeploy":null}`), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.TriggerType != nil {
		t.Errorf("TriggerType = %v, want nil", c.TriggerType)
	}
	if c.AutoDeploy != nil {
		t.Errorf("AutoDeploy = %v, want nil", c.AutoDeploy)
	}
}

// A nil pointer in UpdateComposeRequest must marshal to an explicit JSON
// null (that is what clears the field), and the dialect C strings must
// appear as "" rather than vanishing. Both properties are what `no
// omitempty` buys, and both are invisible without checking the raw body.
func TestUpdateComposeRequestMarshalsNullsAndEmptyStrings(t *testing.T) {
	body, err := json.Marshal(UpdateComposeRequest{ComposeID: "c1"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, k := range []string{"name", "composePath", "command", "suffix", "composeFile", "composeType", "sourceType"} {
		got, ok := raw[k]
		if !ok {
			t.Errorf("%s: absent from the body; dialect C and enum fields must always be transmitted", k)
			continue
		}
		if string(got) != `""` {
			t.Errorf("%s = %s, want \"\" - an explicit null is a 400 on these fields", k, got)
		}
	}

	for _, k := range []string{
		"description", "repository", "owner", "branch", "githubId",
		"customGitUrl", "customGitBranch", "customGitSSHKeyId", "triggerType",
		"autoDeploy", "enableSubmodules", "randomize", "watchPaths",
	} {
		got, ok := raw[k]
		if !ok {
			t.Errorf("%s: absent from the body; an omitted key keeps the stored value, so the field could never be cleared", k)
			continue
		}
		if string(got) != "null" {
			t.Errorf("%s = %s, want null", k, got)
		}
	}
}

// icon and serviceNetworks are v0.30.0 additions to compose.update. Both are
// nullable, so a bare UpdateComposeRequest must marshal them as explicit
// null, never omit them - see doc.go's "serviceNetworks and icon on
// compose.update" section.
func TestUpdateComposeRequestCarriesV030Fields(t *testing.T) {
	raw, err := json.Marshal(UpdateComposeRequest{ComposeID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if string(m["icon"]) != "null" {
		t.Errorf("icon = %s, want null", m["icon"])
	}
	if string(m["serviceNetworks"]) != "null" {
		t.Errorf("serviceNetworks = %s, want null", m["serviceNetworks"])
	}
}

// createEnvFile is a v0.30.0 addition to compose.saveEnvironment. doc.go
// records that an absent key silently keeps the old value there, so the
// field must always reach the wire - a bare request marshals it as explicit
// null.
func TestSaveComposeEnvironmentCarriesCreateEnvFile(t *testing.T) {
	raw, err := json.Marshal(SaveComposeEnvironmentRequest{ComposeID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if string(m["createEnvFile"]) != "null" {
		t.Errorf("createEnvFile = %s, want null", m["createEnvFile"])
	}
}

// compose.deploy carries freshVolumes on every call, as a concrete bool.
func TestDeployComposeCarriesFreshVolumes(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/compose.deploy" {
			t.Errorf("unexpected call: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = fmt.Fprint(w, `{"success":true}`)
	}))
	defer srv.Close()
	c := testClient(t, srv)
	if err := c.DeployCompose(context.Background(), DeployComposeRequest{ComposeID: "c1", FreshVolumes: true}); err != nil {
		t.Fatalf("DeployCompose: %v", err)
	}
	if body["composeId"] != "c1" || body["freshVolumes"] != true {
		t.Errorf("body = %v", body)
	}
	if err := c.DeployCompose(context.Background(), DeployComposeRequest{ComposeID: "c1"}); err != nil {
		t.Fatalf("DeployCompose: %v", err)
	}
	if body["freshVolumes"] != false {
		t.Errorf("freshVolumes = %v, want an explicit false", body["freshVolumes"])
	}
}
