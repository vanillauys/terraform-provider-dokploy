package ai

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func TestFlatten(t *testing.T) {
	a := &client.AI{
		AIID:           "ai-1",
		Name:           "openai",
		APIURL:         "https://api.openai.com/v1",
		APIKey:         "sk-test",
		Model:          "gpt-4o",
		IsEnabled:      true,
		OrganizationID: "org-1",
		CreatedAt:      "2026-09-05T00:00:00Z",
	}
	var m resourceModel
	flatten(a, &m)

	got := map[string]string{
		"id":              m.ID.ValueString(),
		"name":            m.Name.ValueString(),
		"api_url":         m.APIURL.ValueString(),
		"api_key":         m.APIKey.ValueString(),
		"model":           m.Model.ValueString(),
		"organization_id": m.OrganizationID.ValueString(),
		"created_at":      m.CreatedAt.ValueString(),
	}
	want := map[string]string{
		"id":              "ai-1",
		"name":            "openai",
		"api_url":         "https://api.openai.com/v1",
		"api_key":         "sk-test",
		"model":           "gpt-4o",
		"organization_id": "org-1",
		"created_at":      "2026-09-05T00:00:00Z",
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("flatten() %s = %q, want %q", k, got[k], w)
		}
	}
	if !m.IsEnabled.ValueBool() {
		t.Error("flatten() is_enabled = false, want true")
	}
	if !m.APIKeyWo.IsNull() || !m.APIKeyWoVersion.IsNull() {
		t.Errorf("flatten() touched the write-only companions: %v %v", m.APIKeyWo, m.APIKeyWoVersion)
	}
}

// Dokploy v0.31.0 returns the mask for apiKey. flatten must keep the prior
// value, or the apply reports an inconsistent result.
func TestFlattenKeepsKeyWhenMasked(t *testing.T) {
	m := resourceModel{APIKey: types.StringValue("sk-prior")}
	flatten(&client.AI{AIID: "ai-1", APIKey: client.AISecretMask}, &m)
	if m.APIKey.ValueString() != "sk-prior" {
		t.Errorf("api_key = %v, want the prior sk-prior", m.APIKey)
	}
	var imported resourceModel
	flatten(&client.AI{AIID: "ai-1", APIKey: client.AISecretMask}, &imported)
	if !imported.APIKey.IsNull() {
		t.Errorf("imported api_key = %v, want null", imported.APIKey)
	}
}
