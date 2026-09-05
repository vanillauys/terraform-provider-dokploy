package giteaprovider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func TestFlatten(t *testing.T) {
	g := &client.GiteaProvider{
		GiteaID:          "gt-1",
		GiteaURL:         "https://gitea.example.com",
		GiteaInternalURL: "http://gitea.internal",
		RedirectURI:      "https://dokploy.example.com/api/providers/gitea/callback",
		ClientID:         "client-1",
		ClientSecret:     "s3cret",
		GitProviderID:    "gp-1",
		Scopes:           "repo,read:user",
		GitProvider:      client.GitProvider{Name: "main", CreatedAt: "2026-09-05T00:00:00Z"},
	}
	var m resourceModel
	flatten(g, &m)

	got := map[string]string{
		"id":                 m.ID.ValueString(),
		"git_provider_id":    m.GitProviderID.ValueString(),
		"name":               m.Name.ValueString(),
		"gitea_url":          m.GiteaURL.ValueString(),
		"gitea_internal_url": m.GiteaInternalURL.ValueString(),
		"client_id":          m.ClientID.ValueString(),
		"client_secret":      m.ClientSecret.ValueString(),
		"redirect_uri":       m.RedirectURI.ValueString(),
		"scopes":             m.Scopes.ValueString(),
		"created_at":         m.CreatedAt.ValueString(),
	}
	want := map[string]string{
		"id":                 "gt-1",
		"git_provider_id":    "gp-1",
		"name":               "main",
		"gitea_url":          "https://gitea.example.com",
		"gitea_internal_url": "http://gitea.internal",
		"client_id":          "client-1",
		"client_secret":      "s3cret",
		"redirect_uri":       "https://dokploy.example.com/api/providers/gitea/callback",
		"scopes":             "repo,read:user",
		"created_at":         "2026-09-05T00:00:00Z",
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("flatten() %s = %q, want %q", k, got[k], w)
		}
	}
	if !m.ClientSecretWo.IsNull() || !m.ClientSecretWoVersion.IsNull() {
		t.Errorf("flatten() touched the write-only companions: %v %v", m.ClientSecretWo, m.ClientSecretWoVersion)
	}
}

func TestFlatten_emptyInternalURLIsNull(t *testing.T) {
	var m resourceModel
	flatten(&client.GiteaProvider{}, &m)
	if !m.GiteaInternalURL.IsNull() {
		t.Errorf("gitea_internal_url = %v, want null", m.GiteaInternalURL)
	}
}

func TestHideWriteOnly(t *testing.T) {
	m := resourceModel{ClientSecret: types.StringValue("s3cret")}
	hideWriteOnly(&m, map[string]bool{"client_secret": true})
	if !m.ClientSecret.IsNull() {
		t.Errorf("hideWriteOnly() left client_secret = %v", m.ClientSecret)
	}
	m = resourceModel{ClientSecret: types.StringValue("s3cret")}
	hideWriteOnly(&m, nil)
	if m.ClientSecret.IsNull() {
		t.Error("hideWriteOnly() cleared client_secret while no companion is in use")
	}
}
