// Package port holds the dokploy_port data source.
package port

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/datasources/dsutil"
)

type model struct {
	ID            types.String `tfsdk:"id"`
	ApplicationID types.String `tfsdk:"application_id"`
	PublishedPort types.Int64  `tfsdk:"published_port"`
	TargetPort    types.Int64  `tfsdk:"target_port"`
	Protocol      types.String `tfsdk:"protocol"`
	PublishMode   types.String `tfsdk:"publish_mode"`
}

func flatten(p *client.Port, out *model) {
	out.ID = types.StringValue(p.PortID)
	out.ApplicationID = types.StringValue(p.ApplicationID)
	out.PublishedPort = types.Int64Value(p.PublishedPort)
	out.TargetPort = types.Int64Value(p.TargetPort)
	out.Protocol = types.StringValue(p.Protocol)
	out.PublishMode = types.StringValue(p.PublishMode)
}

// NewDataSource returns the dokploy_port data source.
func NewDataSource() datasource.DataSource {
	child := dsutil.Child{
		Kind: "port",
		What: "a published port of an application that already exists in Dokploy",
		Example: "data \"dokploy_port\" \"web\" {\n  application_id = data.dokploy_application.web.id\n" +
			"  published_port = 8080\n}",
	}
	return dsutil.NewDataSource(dsutil.Record[model, client.Port]{
		Name:        "port",
		Description: child.Description(),
		Attributes: map[string]schema.Attribute{
			"id":             child.IDAttribute("application_id"),
			"application_id": dsutil.LookupString("Id of the application that owns the port. Set it with `published_port`."),
			"published_port": dsutil.LookupInt64("Port that the application publishes on the host."),
			"target_port":    dsutil.Int64("Port that the container listens on."),
			"protocol":       dsutil.String("Transport protocol: `tcp` or `udp`."),
			"publish_mode":   dsutil.String("Swarm publish mode: `host` or `ingress`."),
		},
		Validators: dsutil.ChildValidators("application_id", []string{"published_port"}, "published_port"),
		ID:         func(m *model) types.String { return m.ID },
		Get: func(ctx context.Context, c *client.Client, id string) (*client.Port, error) {
			return c.GetPort(ctx, id)
		},
		Find: func(ctx context.Context, c *client.Client, m *model) (*client.Port, error) {
			ports, err := c.ListPorts(ctx, m.ApplicationID.ValueString())
			if err != nil {
				return nil, err
			}
			return dsutil.Find(ports, func(x client.Port) bool { return x.PublishedPort == m.PublishedPort.ValueInt64() },
				"port", "the published port on application "+m.ApplicationID.ValueString())
		},
		Flatten: flatten,
	})()
}
