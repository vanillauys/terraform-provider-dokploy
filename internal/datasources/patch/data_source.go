// Package patch holds the dokploy_patch data source.
package patch

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/datasources/dsutil"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

type model struct {
	ID            types.String `tfsdk:"id"`
	ApplicationID types.String `tfsdk:"application_id"`
	ComposeID     types.String `tfsdk:"compose_id"`
	FilePath      types.String `tfsdk:"file_path"`
	Type          types.String `tfsdk:"type"`
	Content       types.String `tfsdk:"content"`
	Enabled       types.Bool   `tfsdk:"enabled"`
}

func flatten(p *client.Patch, out *model) {
	out.ID = types.StringValue(p.PatchID)
	out.ApplicationID = tfutil.StringOrNull(p.ApplicationID)
	out.ComposeID = tfutil.StringOrNull(p.ComposeID)
	out.FilePath = types.StringValue(p.FilePath)
	out.Type = types.StringValue(p.Type)
	out.Content = types.StringValue(p.Content)
	out.Enabled = types.BoolValue(p.Enabled)
}

// NewDataSource returns the dokploy_patch data source. A lookup sets one
// parent and file_path; Dokploy keeps one patch for each file path of a
// service.
func NewDataSource() datasource.DataSource {
	return dsutil.NewDataSource(dsutil.Record[model, client.Patch]{
		Name: "patch",
		Description: "Looks up a patch of an application or a compose service that already exists in Dokploy:\n\n" +
			"```terraform\ndata \"dokploy_patch\" \"config\" {\n  application_id = data.dokploy_application.web.id\n" +
			"  file_path      = \"config/app.yaml\"\n}\n```",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Optional: true, Computed: true, Description: "Patch id. Set it for a lookup by id, or set a parent and `file_path`."},
			"application_id": dsutil.LookupString("Id of the application that owns the patch. Set it, or `compose_id`, with `file_path`."),
			"compose_id":     dsutil.LookupString("Id of the compose service that owns the patch. Set it, or `application_id`, with `file_path`."),
			"file_path":      dsutil.LookupString("Path of the file, relative to the repository root."),
			"type":           dsutil.String("What the patch does to the file: `create`, `update`, or `delete`."),
			"content":        dsutil.String("Content of the file, as Dokploy stores it."),
			"enabled":        dsutil.Bool("Whether Dokploy applies the patch at the next build."),
		},
		Validators: []datasource.ConfigValidator{
			datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("application_id"), path.MatchRoot("compose_id")),
			datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("file_path")),
		},
		ID: func(m *model) types.String { return m.ID },
		Get: func(ctx context.Context, c *client.Client, id string) (*client.Patch, error) {
			return c.GetPatch(ctx, id)
		},
		Find: func(ctx context.Context, c *client.Client, m *model) (*client.Patch, error) {
			entityID, entityType := m.ApplicationID.ValueString(), "application"
			if !m.ComposeID.IsNull() {
				entityID, entityType = m.ComposeID.ValueString(), "compose"
			}
			patches, err := c.ListPatches(ctx, entityID, entityType)
			if err != nil {
				return nil, err
			}
			return dsutil.Find(patches, func(p client.Patch) bool {
				return p.FilePath == m.FilePath.ValueString()
			}, "patch", "the file path on "+entityType+" "+entityID)
		},
		Flatten: flatten,
	})()
}
