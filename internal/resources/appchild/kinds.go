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
		Description: "A published port on a Dokploy application, mapping a host port to a container port.",
		Attributes: map[string]schema.Attribute{
			"published_port": schema.Int64Attribute{
				Required:    true,
				Description: "Port published on the host.",
			},
			"target_port": schema.Int64Attribute{
				Required:    true,
				Description: "Port the container listens on.",
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
			"and `redirects.create` does not return the record it made, so this provider identifies a " +
			"newly created redirect by diffing the application's redirect list around the call. " +
			"Creating redirects through the Dokploy UI while an apply is running can make that ambiguous, " +
			"and the provider errors rather than guessing.",
		Attributes: map[string]schema.Attribute{
			"regex": schema.StringAttribute{
				Required:    true,
				Description: "Path regex to match, e.g. `^/old/(.*)`.",
			},
			"replacement": schema.StringAttribute{
				Required:    true,
				Description: "Replacement path, e.g. `/new/$1`.",
			},
			"permanent": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				Description: "Issue a permanent (308) redirect rather than a temporary (307) one.",
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
}

func flattenSecurity(s *client.Security, m *SecurityModel) {
	m.ID = types.StringValue(s.SecurityID)
	m.ApplicationID = types.StringValue(s.ApplicationID)
	m.Username = types.StringValue(s.Username)
	m.Password = types.StringValue(s.Password)
}

func SecurityKind() Kind[SecurityModel] {
	return Kind[SecurityModel]{
		Name: "security",
		Description: "HTTP basic-auth credentials protecting a Dokploy application.\n\n" +
			"~> Dokploy stores and returns `password` in cleartext. The attribute is marked sensitive so " +
			"Terraform will not print it, but it is readable by anyone with API access to the instance, " +
			"and it is written to Terraform state in cleartext like any other attribute.",
		Attributes: map[string]schema.Attribute{
			"username": schema.StringAttribute{
				Required:    true,
				Description: "Basic-auth username.",
			},
			"password": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "Basic-auth password.",
			},
		},
		ID:    func(m *SecurityModel) string { return m.ID.ValueString() },
		SetID: func(m *SecurityModel, id string) { m.ID = types.StringValue(id) },
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
	}
}
