// Package redirect holds the dokploy_redirect data source.
package redirect

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
	Regex         types.String `tfsdk:"regex"`
	Replacement   types.String `tfsdk:"replacement"`
	Permanent     types.Bool   `tfsdk:"permanent"`
}

func flatten(r *client.Redirect, out *model) {
	out.ID = types.StringValue(r.RedirectID)
	out.ApplicationID = types.StringValue(r.ApplicationID)
	out.Regex = types.StringValue(r.Regex)
	out.Replacement = types.StringValue(r.Replacement)
	out.Permanent = types.BoolValue(r.Permanent)
}

// NewDataSource returns the dokploy_redirect data source.
func NewDataSource() datasource.DataSource {
	child := dsutil.Child{
		Kind: "redirect",
		What: "a Traefik regex redirect of an application that already exists in Dokploy",
		Example: "data \"dokploy_redirect\" \"old\" {\n  application_id = data.dokploy_application.web.id\n" +
			"  regex          = \"^/old/(.*)\"\n}",
	}
	return dsutil.NewDataSource(dsutil.Record[model, client.Redirect]{
		Name:        "redirect",
		Description: child.Description(),
		Attributes: map[string]schema.Attribute{
			"id":             child.IDAttribute("application_id"),
			"application_id": dsutil.LookupString("Id of the application that owns the redirect. Set it with `regex`."),
			"regex":          dsutil.LookupString("Path regex that the redirect matches, for example `^/old/(.*)`."),
			"replacement":    dsutil.String("Replacement path."),
			"permanent":      dsutil.Bool("Whether the redirect is permanent (308) instead of temporary (307)."),
		},
		Validators: dsutil.ChildValidators("application_id", []string{"regex"}, "regex"),
		ID:         func(m *model) types.String { return m.ID },
		Get: func(ctx context.Context, c *client.Client, id string) (*client.Redirect, error) {
			return c.GetRedirect(ctx, id)
		},
		Find: func(ctx context.Context, c *client.Client, m *model) (*client.Redirect, error) {
			redirects, err := c.ListRedirects(ctx, m.ApplicationID.ValueString())
			if err != nil {
				return nil, err
			}
			return dsutil.Find(redirects, func(x client.Redirect) bool { return x.Regex == m.Regex.ValueString() },
				"redirect", "the regex on application "+m.ApplicationID.ValueString())
		},
		Flatten: flatten,
	})()
}
