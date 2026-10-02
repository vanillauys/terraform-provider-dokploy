package webserversettings

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

type resourceModel struct {
	ID                  types.String `tfsdk:"id"`
	ServerIP            types.String `tfsdk:"server_ip"`
	Host                types.String `tfsdk:"host"`
	HTTPS               types.Bool   `tfsdk:"https"`
	CertificateType     types.String `tfsdk:"certificate_type"`
	LetsEncryptEmail    types.String `tfsdk:"lets_encrypt_email"`
	EnableDockerCleanup types.Bool   `tfsdk:"enable_docker_cleanup"`
	LogCleanupCron      types.String `tfsdk:"log_cleanup_cron"`
	BuildsConcurrency   types.Int64  `tfsdk:"builds_concurrency"`
}

// flatten maps the server row to the model. host and letsEncryptEmail read
// null on a fresh install and "" after a clear; both are null here. A null
// logCleanupCron means the cleanup is off, which the attribute spells "".
func flatten(s *client.WebServerSettings, out *resourceModel) {
	out.ID = types.StringValue(s.ID)
	out.ServerIP = tfutil.StringOrNull(s.ServerIP)
	out.Host = tfutil.StringOrNull(s.Host)
	out.HTTPS = types.BoolValue(s.HTTPS)
	out.CertificateType = types.StringValue(s.CertificateType)
	out.LetsEncryptEmail = tfutil.StringOrNull(s.LetsEncryptEmail)
	out.EnableDockerCleanup = types.BoolValue(s.EnableDockerCleanup)
	if s.LogCleanupCron != nil {
		out.LogCleanupCron = types.StringValue(*s.LogCleanupCron)
	} else {
		out.LogCleanupCron = types.StringValue("")
	}
	out.BuildsConcurrency = types.Int64Value(s.BuildsConcurrency)
}

// domainChanged reports whether the plan differs from the server in the
// fields that settings.assignDomainServer writes. That endpoint also
// rewrites the Traefik route of the dashboard, so the resource calls it
// only for a real change.
func domainChanged(plan, have resourceModel) bool {
	return !plan.Host.Equal(have.Host) || !plan.HTTPS.Equal(have.HTTPS) ||
		!plan.CertificateType.Equal(have.CertificateType) || !plan.LetsEncryptEmail.Equal(have.LetsEncryptEmail)
}

func domainRequest(m resourceModel) client.AssignWebServerDomainRequest {
	return client.AssignWebServerDomainRequest{
		Host:             m.Host.ValueString(),
		CertificateType:  m.CertificateType.ValueString(),
		LetsEncryptEmail: m.LetsEncryptEmail.ValueStringPointer(),
		HTTPS:            m.HTTPS.ValueBool(),
	}
}

func logCleanupRequest(m resourceModel) client.UpdateLogCleanupRequest {
	if m.LogCleanupCron.ValueString() == "" {
		return client.UpdateLogCleanupRequest{}
	}
	return client.UpdateLogCleanupRequest{CronExpression: m.LogCleanupCron.ValueStringPointer()}
}
