package destination

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func TestFlatten(t *testing.T) {
	ctx := context.Background()
	d := &client.Destination{
		DestinationID:   "dest-1",
		Name:            "backups",
		Provider:        "Cloudflare",
		Endpoint:        "https://account.r2.cloudflarestorage.com",
		Bucket:          "backups",
		Region:          "auto",
		AccessKey:       "ak",
		SecretAccessKey: "sk",
		AdditionalFlags: []string{"--no-check-certificate"},
		CreatedAt:       "2026-09-05T00:00:00Z",
	}
	var m resourceModel
	var diags diag.Diagnostics
	flatten(ctx, d, &m, &diags)
	if diags.HasError() {
		t.Fatalf("flatten(): %v", diags)
	}

	got := map[string]string{
		"id":                m.ID.ValueString(),
		"name":              m.Name.ValueString(),
		"provider_name":     m.Provider.ValueString(),
		"endpoint":          m.Endpoint.ValueString(),
		"bucket":            m.Bucket.ValueString(),
		"region":            m.Region.ValueString(),
		"access_key":        m.AccessKey.ValueString(),
		"secret_access_key": m.SecretAccessKey.ValueString(),
		"created_at":        m.CreatedAt.ValueString(),
	}
	want := map[string]string{
		"id":                "dest-1",
		"name":              "backups",
		"provider_name":     "Cloudflare",
		"endpoint":          "https://account.r2.cloudflarestorage.com",
		"bucket":            "backups",
		"region":            "auto",
		"access_key":        "ak",
		"secret_access_key": "sk",
		"created_at":        "2026-09-05T00:00:00Z",
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("flatten() %s = %q, want %q", k, got[k], w)
		}
	}
	var flags []string
	if d := m.AdditionalFlags.ElementsAs(ctx, &flags, false); d.HasError() {
		t.Fatalf("additional_flags: %v", d)
	}
	if len(flags) != 1 || flags[0] != "--no-check-certificate" {
		t.Errorf("additional_flags = %v, want [--no-check-certificate]", flags)
	}
	if !m.AccessKeyWo.IsNull() || !m.SecretAccessKeyWo.IsNull() {
		t.Errorf("flatten() touched the write-only companions: %v %v", m.AccessKeyWo, m.SecretAccessKeyWo)
	}
}

// The server stores [] when no flag is set; a null list would never
// converge with the Optional+Computed default.
func TestFlagsValue_nilIsEmptyList(t *testing.T) {
	var diags diag.Diagnostics
	list := flagsValue(context.Background(), nil, &diags)
	if diags.HasError() {
		t.Fatalf("flagsValue(): %v", diags)
	}
	if list.IsNull() || list.IsUnknown() || len(list.Elements()) != 0 {
		t.Errorf("flagsValue(nil) = %v, want an empty known list", list)
	}
}

func TestFlagsRequest(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	if got := flagsRequest(ctx, types.ListNull(types.StringType), &diags); got == nil || len(got) != 0 {
		t.Errorf("flagsRequest(null) = %#v, want an empty non-nil slice", got)
	}
	if got := flagsRequest(ctx, types.ListUnknown(types.StringType), &diags); got == nil || len(got) != 0 {
		t.Errorf("flagsRequest(unknown) = %#v, want an empty non-nil slice", got)
	}
	list, _ := types.ListValueFrom(ctx, types.StringType, []string{"a", "b"})
	got := flagsRequest(ctx, list, &diags)
	if diags.HasError() {
		t.Fatalf("flagsRequest(): %v", diags)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("flagsRequest([a b]) = %v", got)
	}
}
