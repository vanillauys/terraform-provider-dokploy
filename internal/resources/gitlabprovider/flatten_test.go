package gitlabprovider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func TestFlatten(t *testing.T) {
	g := &client.GitlabProvider{
		GitlabID:          "gl-1",
		GitlabURL:         "https://gitlab.com",
		GitlabInternalURL: "http://gitlab.internal",
		ApplicationID:     "app-1",
		RedirectURI:       "https://dokploy.example.com/api/providers/gitlab/callback",
		Secret:            "s3cret",
		GroupName:         "my-group",
		GitProviderID:     "gp-1",
		GitProvider:       client.GitProvider{Name: "main", CreatedAt: "2026-09-05T00:00:00Z"},
	}
	var m resourceModel
	flatten(g, &m)

	got := map[string]string{
		"id":                  m.ID.ValueString(),
		"git_provider_id":     m.GitProviderID.ValueString(),
		"name":                m.Name.ValueString(),
		"gitlab_url":          m.GitlabURL.ValueString(),
		"gitlab_internal_url": m.GitlabInternalURL.ValueString(),
		"application_id":      m.ApplicationID.ValueString(),
		"secret":              m.Secret.ValueString(),
		"group_name":          m.GroupName.ValueString(),
		"redirect_uri":        m.RedirectURI.ValueString(),
		"created_at":          m.CreatedAt.ValueString(),
	}
	want := map[string]string{
		"id":                  "gl-1",
		"git_provider_id":     "gp-1",
		"name":                "main",
		"gitlab_url":          "https://gitlab.com",
		"gitlab_internal_url": "http://gitlab.internal",
		"application_id":      "app-1",
		"secret":              "s3cret",
		"group_name":          "my-group",
		"redirect_uri":        "https://dokploy.example.com/api/providers/gitlab/callback",
		"created_at":          "2026-09-05T00:00:00Z",
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("flatten() %s = %q, want %q", k, got[k], w)
		}
	}
	if !m.SecretWo.IsNull() || !m.SecretWoVersion.IsNull() {
		t.Errorf("flatten() touched the write-only companions: %v %v", m.SecretWo, m.SecretWoVersion)
	}
}

func TestFlatten_emptyOptionalsAreNull(t *testing.T) {
	var m resourceModel
	flatten(&client.GitlabProvider{}, &m)
	if !m.GitlabInternalURL.IsNull() {
		t.Errorf("gitlab_internal_url = %v, want null", m.GitlabInternalURL)
	}
	if !m.GroupName.IsNull() {
		t.Errorf("group_name = %v, want null", m.GroupName)
	}
}

func TestHideWriteOnly(t *testing.T) {
	m := resourceModel{Secret: types.StringValue("s3cret")}
	hideWriteOnly(&m, map[string]bool{"secret": true})
	if !m.Secret.IsNull() {
		t.Errorf("hideWriteOnly() left secret = %v", m.Secret)
	}
	m = resourceModel{Secret: types.StringValue("s3cret")}
	hideWriteOnly(&m, nil)
	if m.Secret.IsNull() {
		t.Error("hideWriteOnly() cleared secret while no companion is in use")
	}
}
