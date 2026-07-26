package domain

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// domain_type is derived, never taken from config: the two user-facing values
// are fully determined by which attachment id is set.
func TestDomainTypeFor(t *testing.T) {
	app := &resourceModel{ApplicationID: types.StringValue("a1"), ComposeID: types.StringNull()}
	if got := domainTypeFor(app); got != "application" {
		t.Errorf("with application_id set, domainTypeFor = %q, want application", got)
	}
	compose := &resourceModel{ApplicationID: types.StringNull(), ComposeID: types.StringValue("c1")}
	if got := domainTypeFor(compose); got != "compose" {
		t.Errorf("with compose_id set, domainTypeFor = %q, want compose", got)
	}
}

func TestFlattenMapsNullPointersToNullValues(t *testing.T) {
	appID := "a1"
	var m resourceModel
	diags := flatten(context.Background(), &client.Domain{
		DomainID:           "d1",
		Host:               "app.example.com",
		Path:               "/",
		InternalPath:       "/",
		Port:               3000,
		HTTPS:              false,
		StripPath:          false,
		CertificateType:    "none",
		CustomCertResolver: nil,
		CustomEntrypoint:   nil,
		ServiceName:        nil,
		ForwardAuthEnabled: false,
		Middlewares:        []string{},
		DomainType:         "application",
		UniqueConfigKey:    1,
		ApplicationID:      &appID,
		ComposeID:          nil,
		CreatedAt:          "2026-07-26T16:41:14.242Z",
	}, &m)
	if diags.HasError() {
		t.Fatalf("flatten produced errors: %v", diags)
	}
	if m.ID.ValueString() != "d1" {
		t.Errorf("ID = %v, want d1", m.ID)
	}
	if !m.CustomEntrypoint.IsNull() || !m.CustomCertResolver.IsNull() || !m.ServiceName.IsNull() {
		t.Error("nil pointers must flatten to null Terraform values")
	}
	if m.ApplicationID.ValueString() != "a1" {
		t.Errorf("ApplicationID = %v, want a1", m.ApplicationID)
	}
	if !m.ComposeID.IsNull() {
		t.Errorf("ComposeID = %v, want null", m.ComposeID)
	}
	if m.Port.ValueInt64() != 3000 {
		t.Errorf("Port = %v, want 3000", m.Port)
	}
}
