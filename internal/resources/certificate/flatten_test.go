package certificate

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func TestFlatten(t *testing.T) {
	c := &client.Certificate{
		CertificateID:   "cert-1",
		Name:            "wildcard",
		CertificateData: "-----BEGIN CERTIFICATE-----",
		PrivateKey:      "pem",
		CertificatePath: "/etc/dokploy/traefik/dynamic/certificates/cert-1",
		AutoRenew:       true,
		OrganizationID:  "org-1",
		ServerID:        "srv-1",
	}
	var m resourceModel
	flatten(c, &m)

	got := map[string]string{
		"id":               m.ID.ValueString(),
		"name":             m.Name.ValueString(),
		"certificate_data": m.CertificateData.ValueString(),
		"private_key":      m.PrivateKey.ValueString(),
		"certificate_path": m.CertificatePath.ValueString(),
		"organization_id":  m.OrganizationID.ValueString(),
		"server_id":        m.ServerID.ValueString(),
	}
	want := map[string]string{
		"id":               "cert-1",
		"name":             "wildcard",
		"certificate_data": "-----BEGIN CERTIFICATE-----",
		"private_key":      "pem",
		"certificate_path": "/etc/dokploy/traefik/dynamic/certificates/cert-1",
		"organization_id":  "org-1",
		"server_id":        "srv-1",
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("flatten() %s = %q, want %q", k, got[k], w)
		}
	}
	if !m.AutoRenew.ValueBool() {
		t.Error("flatten() auto_renew = false, want true")
	}
	if !m.PrivateKeyWo.IsNull() || !m.PrivateKeyWoVersion.IsNull() {
		t.Errorf("flatten() touched the write-only companions: %v %v", m.PrivateKeyWo, m.PrivateKeyWoVersion)
	}
}

func TestFlatten_localServerIsNull(t *testing.T) {
	var m resourceModel
	flatten(&client.Certificate{CertificateID: "cert-1"}, &m)
	if !m.ServerID.IsNull() {
		t.Errorf("server_id = %v, want null for a certificate on the Dokploy host", m.ServerID)
	}
}

func TestServerRequest(t *testing.T) {
	if got := serverRequest(types.StringNull()); got != nil {
		t.Errorf("serverRequest(null) = %q, want nil", *got)
	}
	if got := serverRequest(types.StringUnknown()); got != nil {
		t.Errorf("serverRequest(unknown) = %q, want nil", *got)
	}
	got := serverRequest(types.StringValue("srv-1"))
	if got == nil || *got != "srv-1" {
		t.Errorf("serverRequest(srv-1) = %v, want srv-1", got)
	}
}
