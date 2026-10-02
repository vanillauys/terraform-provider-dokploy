package webserversettings

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func strPtr(s string) *string { return &s }

// host and letsEncryptEmail read null on a fresh install and "" after a
// clear. Both must flatten to null, or a config without them plans a diff.
func TestFlattenEmptyAndNull(t *testing.T) {
	for _, v := range []*string{nil, strPtr("")} {
		var m resourceModel
		flatten(&client.WebServerSettings{ID: "w", Host: v, LetsEncryptEmail: v, CertificateType: "none", BuildsConcurrency: 1}, &m)
		if !m.Host.IsNull() || !m.LetsEncryptEmail.IsNull() {
			t.Errorf("host/email = %v/%v, want null/null", m.Host, m.LetsEncryptEmail)
		}
		if m.LogCleanupCron.ValueString() != "" || m.LogCleanupCron.IsNull() {
			t.Errorf("a null logCleanupCron flattened to %v, want \"\"", m.LogCleanupCron)
		}
	}
}

func TestFlattenValues(t *testing.T) {
	var m resourceModel
	flatten(&client.WebServerSettings{
		ID: "w", ServerIP: strPtr("10.0.0.9"), CertificateType: "letsencrypt", HTTPS: true,
		Host: strPtr("d.example.com"), LetsEncryptEmail: strPtr("a@example.com"),
		EnableDockerCleanup: true, LogCleanupCron: strPtr("*/5 * * * *"), BuildsConcurrency: 3,
	}, &m)
	want := resourceModel{
		ID: types.StringValue("w"), ServerIP: types.StringValue("10.0.0.9"),
		Host: types.StringValue("d.example.com"), HTTPS: types.BoolValue(true),
		CertificateType: types.StringValue("letsencrypt"), LetsEncryptEmail: types.StringValue("a@example.com"),
		EnableDockerCleanup: types.BoolValue(true), LogCleanupCron: types.StringValue("*/5 * * * *"),
		BuildsConcurrency: types.Int64Value(3),
	}
	if m != want {
		t.Errorf("flatten = %+v, want %+v", m, want)
	}
}

// "" stops the log cleanup, and the server stops it only for a null.
func TestLogCleanupRequest(t *testing.T) {
	if r := logCleanupRequest(resourceModel{LogCleanupCron: types.StringValue("")}); r.CronExpression != nil {
		t.Errorf("\"\" sent %q, want null", *r.CronExpression)
	}
	r := logCleanupRequest(resourceModel{LogCleanupCron: types.StringValue("0 1 * * *")})
	if r.CronExpression == nil || *r.CronExpression != "0 1 * * *" {
		t.Errorf("cron sent %v, want 0 1 * * *", r.CronExpression)
	}
}

// A null host must reach the wire as "", because assignDomainServer rejects
// a null host, and a null email as null, which clears it.
func TestDomainRequestClears(t *testing.T) {
	r := domainRequest(resourceModel{
		Host: types.StringNull(), LetsEncryptEmail: types.StringNull(),
		CertificateType: types.StringValue("none"), HTTPS: types.BoolValue(false),
	})
	if r.Host != "" || r.LetsEncryptEmail != nil || r.CertificateType != "none" || r.HTTPS {
		t.Errorf("domainRequest = %+v", r)
	}
}

func TestDomainChanged(t *testing.T) {
	base := resourceModel{
		Host: types.StringNull(), HTTPS: types.BoolValue(false),
		CertificateType: types.StringValue("none"), LetsEncryptEmail: types.StringNull(),
	}
	if domainChanged(base, base) {
		t.Error("an equal model reported a change")
	}
	changed := base
	changed.LetsEncryptEmail = types.StringValue("a@example.com")
	if !domainChanged(changed, base) {
		t.Error("an email change was not reported")
	}
}
