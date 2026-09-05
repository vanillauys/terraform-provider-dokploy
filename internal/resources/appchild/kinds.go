package appchild

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

// ---------------------------------------------------------------- port

type PortModel struct {
	ID            types.String `tfsdk:"id"`
	ApplicationID types.String `tfsdk:"application_id"`
	PublishedPort types.Int64  `tfsdk:"published_port"`
	TargetPort    types.Int64  `tfsdk:"target_port"`
	Protocol      types.String `tfsdk:"protocol"`
	PublishMode   types.String `tfsdk:"publish_mode"`
}

func flattenPort(p *client.Port, m *PortModel) {
	m.ID = types.StringValue(p.PortID)
	m.ApplicationID = types.StringValue(p.ApplicationID)
	m.PublishedPort = types.Int64Value(p.PublishedPort)
	m.TargetPort = types.Int64Value(p.TargetPort)
	m.Protocol = types.StringValue(p.Protocol)
	m.PublishMode = types.StringValue(p.PublishMode)
}

func PortKind() Kind[PortModel] {
	return Kind[PortModel]{
		Name:        "port",
		Description: "A published port on a Dokploy application. It maps a host port to a container port.",
		Attributes: map[string]schema.Attribute{
			"published_port": schema.Int64Attribute{
				Required:    true,
				Description: "Port published on the host.",
			},
			"target_port": schema.Int64Attribute{
				Required:    true,
				Description: "Port that the container listens on.",
			},
			"protocol": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("tcp"),
				Description: "Transport protocol: `tcp` or `udp`.",
				Validators:  []validator.String{stringvalidator.OneOf(client.PortProtocols...)},
			},
			"publish_mode": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("host"),
				Description: "Swarm publish mode: `host` or `ingress`.",
				Validators:  []validator.String{stringvalidator.OneOf(client.PortPublishMode...)},
			},
		},
		ID:    func(m *PortModel) string { return m.ID.ValueString() },
		SetID: func(m *PortModel, id string) { m.ID = types.StringValue(id) },
		Create: func(ctx context.Context, c *client.Client, m *PortModel) error {
			p, err := c.CreatePort(ctx, client.CreatePortRequest{
				ApplicationID: m.ApplicationID.ValueString(),
				PublishedPort: m.PublishedPort.ValueInt64(),
				TargetPort:    m.TargetPort.ValueInt64(),
				Protocol:      m.Protocol.ValueString(),
				PublishMode:   m.PublishMode.ValueString(),
			})
			if err != nil {
				return err
			}
			flattenPort(p, m)
			return nil
		},
		Read: func(ctx context.Context, c *client.Client, m *PortModel) error {
			p, err := c.GetPort(ctx, m.ID.ValueString())
			if err != nil {
				return err
			}
			flattenPort(p, m)
			return nil
		},
		Update: func(ctx context.Context, c *client.Client, m *PortModel) error {
			return c.UpdatePort(ctx, client.UpdatePortRequest{
				PortID:        m.ID.ValueString(),
				PublishedPort: m.PublishedPort.ValueInt64(),
				TargetPort:    m.TargetPort.ValueInt64(),
				Protocol:      m.Protocol.ValueString(),
				PublishMode:   m.PublishMode.ValueString(),
			})
		},
		Delete: func(ctx context.Context, c *client.Client, id string) error {
			return c.DeletePort(ctx, id)
		},
	}
}

// ---------------------------------------------------------------- redirect

type RedirectModel struct {
	ID            types.String `tfsdk:"id"`
	ApplicationID types.String `tfsdk:"application_id"`
	Regex         types.String `tfsdk:"regex"`
	Replacement   types.String `tfsdk:"replacement"`
	Permanent     types.Bool   `tfsdk:"permanent"`
}

func flattenRedirect(r *client.Redirect, m *RedirectModel) {
	m.ID = types.StringValue(r.RedirectID)
	m.ApplicationID = types.StringValue(r.ApplicationID)
	m.Regex = types.StringValue(r.Regex)
	m.Replacement = types.StringValue(r.Replacement)
	m.Permanent = types.BoolValue(r.Permanent)
}

func RedirectKind() Kind[RedirectModel] {
	return Kind[RedirectModel]{
		Name: "redirect",
		Description: "A Traefik regex redirect on a Dokploy application.\n\n" +
			"~> Redirects are not unique. Dokploy allows two identical redirects on one application, " +
			"and `redirects.create` does not return the new record. The provider therefore identifies a " +
			"new redirect from a comparison of the redirect list of the application before and after the call. " +
			"A redirect from the Dokploy UI during an apply can make that comparison ambiguous, " +
			"and the provider errors rather than guessing.",
		Attributes: map[string]schema.Attribute{
			"regex": schema.StringAttribute{
				Required:    true,
				Description: "Path regex to match, for example `^/old/(.*)`.",
			},
			"replacement": schema.StringAttribute{
				Required:    true,
				Description: "Replacement path, for example `/new/$1`.",
			},
			"permanent": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				Description: "Issue a permanent redirect (308) instead of a temporary one (307).",
			},
		},
		ID:    func(m *RedirectModel) string { return m.ID.ValueString() },
		SetID: func(m *RedirectModel, id string) { m.ID = types.StringValue(id) },
		Create: func(ctx context.Context, c *client.Client, m *RedirectModel) error {
			r, err := c.CreateRedirect(ctx, client.CreateRedirectRequest{
				ApplicationID: m.ApplicationID.ValueString(),
				Regex:         m.Regex.ValueString(),
				Replacement:   m.Replacement.ValueString(),
				Permanent:     m.Permanent.ValueBool(),
			})
			if err != nil {
				return err
			}
			flattenRedirect(r, m)
			return nil
		},
		Read: func(ctx context.Context, c *client.Client, m *RedirectModel) error {
			r, err := c.GetRedirect(ctx, m.ID.ValueString())
			if err != nil {
				return err
			}
			flattenRedirect(r, m)
			return nil
		},
		Update: func(ctx context.Context, c *client.Client, m *RedirectModel) error {
			return c.UpdateRedirect(ctx, client.UpdateRedirectRequest{
				RedirectID:  m.ID.ValueString(),
				Regex:       m.Regex.ValueString(),
				Replacement: m.Replacement.ValueString(),
				Permanent:   m.Permanent.ValueBool(),
			})
		},
		Delete: func(ctx context.Context, c *client.Client, id string) error {
			return c.DeleteRedirect(ctx, id)
		},
	}
}

// ---------------------------------------------------------------- security

type SecurityModel struct {
	ID            types.String `tfsdk:"id"`
	ApplicationID types.String `tfsdk:"application_id"`
	Username      types.String `tfsdk:"username"`
	Password      types.String `tfsdk:"password"`
	// The write-only companions (tfutil.WriteOnlyCompanions). Only the
	// config carries the _wo value; the plan and the state hold null for it.
	PasswordWo        types.String `tfsdk:"password_wo"`
	PasswordWoVersion types.Int64  `tfsdk:"password_wo_version"`
}

func flattenSecurity(s *client.Security, m *SecurityModel) {
	m.ID = types.StringValue(s.SecurityID)
	m.ApplicationID = types.StringValue(s.ApplicationID)
	m.Username = types.StringValue(s.Username)
	m.Password = types.StringValue(s.Password)
}

func SecurityKind() Kind[SecurityModel] {
	attrs := map[string]schema.Attribute{
		"username": schema.StringAttribute{
			Required:    true,
			Description: "Basic-auth username.",
		},
		// password is Optional, not Required, only because its write-only
		// companion can replace it; the ExactlyOneOf validator on the
		// companion still demands one of the two.
		"password": schema.StringAttribute{
			Optional:    true,
			Sensitive:   true,
			Description: "Basic-auth password. Set this attribute or `password_wo`.",
		},
	}
	for name, attr := range tfutil.WriteOnlyCompanions("password", tfutil.WriteOnlyOptions{ExactlyOne: true}) {
		attrs[name] = attr
	}
	return Kind[SecurityModel]{
		Name: "security",
		Description: "HTTP basic-auth credentials that protect a Dokploy application.\n\n" +
			"~> Dokploy stores and returns `password` in cleartext. The attribute is sensitive, so " +
			"Terraform does not print it, but anyone with API access to the server can read it, " +
			"and the Terraform state holds it in cleartext like any other attribute. " +
			"The `password_wo` companion keeps it out of the state.",
		Attributes: attrs,
		ID:         func(m *SecurityModel) string { return m.ID.ValueString() },
		SetID:      func(m *SecurityModel, id string) { m.ID = types.StringValue(id) },
		Create: func(ctx context.Context, c *client.Client, m *SecurityModel) error {
			s, err := c.CreateSecurity(ctx, client.CreateSecurityRequest{
				ApplicationID: m.ApplicationID.ValueString(),
				Username:      m.Username.ValueString(),
				Password:      m.Password.ValueString(),
			})
			if err != nil {
				return err
			}
			flattenSecurity(s, m)
			return nil
		},
		Read: func(ctx context.Context, c *client.Client, m *SecurityModel) error {
			s, err := c.GetSecurity(ctx, m.ID.ValueString())
			if err != nil {
				return err
			}
			flattenSecurity(s, m)
			return nil
		},
		Update: func(ctx context.Context, c *client.Client, m *SecurityModel) error {
			return c.UpdateSecurity(ctx, client.UpdateSecurityRequest{
				SecurityID: m.ID.ValueString(),
				Username:   m.Username.ValueString(),
				Password:   m.Password.ValueString(),
			})
		},
		Delete: func(ctx context.Context, c *client.Client, id string) error {
			return c.DeleteSecurity(ctx, id)
		},
		Secrets: []string{"password"},
		// ResolveSecrets leaves the value to send in m.Password: Create and
		// Update above read it from there. HideSecret nulls it again after
		// the call, whenever the companion is in use.
		ResolveSecrets: func(ctx context.Context, c *client.Client, plan, cfg, prior *SecurityModel) (map[string]bool, error) {
			inUse := map[string]bool{"password": !cfg.PasswordWo.IsNull()}
			if prior == nil {
				plan.Password = types.StringValue(tfutil.SecretToCreate(plan.Password, cfg.PasswordWo))
				return inUse, nil
			}
			value, send := tfutil.SecretToUpdate(plan.Password, cfg.PasswordWo, prior.Password, plan.PasswordWoVersion, prior.PasswordWoVersion)
			if !send {
				// security.update needs the full field set (client/
				// security.go), so a write-only password with nothing new
				// to send resends the stored one, which the API returns in
				// cleartext.
				s, err := c.GetSecurity(ctx, plan.ID.ValueString())
				if err != nil {
					return nil, err
				}
				value = s.Password
			}
			plan.Password = types.StringValue(value)
			return inUse, nil
		},
		HideSecret: func(m *SecurityModel, _ string) { m.Password = types.StringNull() },
	}
}
