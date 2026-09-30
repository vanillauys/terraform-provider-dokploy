// Package ai holds the dokploy_ai data source.
package ai

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/datasources/dsutil"
)

// model has no api_key: the data source never reads the key into a value.
type model struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	APIURL         types.String `tfsdk:"api_url"`
	Model          types.String `tfsdk:"model"`
	IsEnabled      types.Bool   `tfsdk:"is_enabled"`
	OrganizationID types.String `tfsdk:"organization_id"`
	CreatedAt      types.String `tfsdk:"created_at"`
}

func flatten(a *client.AI, out *model) {
	out.ID = types.StringValue(a.AIID)
	out.Name = types.StringValue(a.Name)
	out.APIURL = types.StringValue(a.APIURL)
	out.Model = types.StringValue(a.Model)
	out.IsEnabled = types.BoolValue(a.IsEnabled)
	out.OrganizationID = types.StringValue(a.OrganizationID)
	out.CreatedAt = types.StringValue(a.CreatedAt)
}

// NewDataSource returns the dokploy_ai data source.
func NewDataSource() datasource.DataSource {
	l := dsutil.Lookup{
		Kind: "AI provider configuration", Plural: "AI provider configurations",
		What:    "an AI provider configuration that already exists in Dokploy (Settings > AI)",
		Example: "data \"dokploy_ai\" \"main\" {\n  name = \"openai\"\n}",
		Secret:  "the API key", SecretAttr: "The attribute `api_key`", Resource: "`dokploy_ai`",
	}
	attrs := l.Attributes()
	attrs["api_url"] = dsutil.String("Base URL of the OpenAI-compatible endpoint.")
	attrs["model"] = dsutil.String("Model that Dokploy requests.")
	attrs["is_enabled"] = dsutil.Bool("Whether Dokploy uses the configuration.")
	attrs["organization_id"] = dsutil.String("Id of the organization that owns the configuration.")
	attrs["created_at"] = dsutil.String("Creation timestamp from the server.")
	return dsutil.NewDataSource(dsutil.Record[model, client.AI]{
		Name:        "ai",
		Description: l.Description(),
		Attributes:  attrs,
		Validators:  dsutil.IDOrName(),
		ID:          func(m *model) types.String { return m.ID },
		Get: func(ctx context.Context, c *client.Client, id string) (*client.AI, error) {
			return c.GetAI(ctx, id)
		},
		Find: func(ctx context.Context, c *client.Client, m *model) (*client.AI, error) {
			all, err := c.ListAIs(ctx)
			if err != nil {
				return nil, err
			}
			return dsutil.Find(all, func(x client.AI) bool { return x.Name == m.Name.ValueString() },
				"AI provider configuration", "the name "+m.Name.ValueString())
		},
		Flatten: flatten,
	})()
}
