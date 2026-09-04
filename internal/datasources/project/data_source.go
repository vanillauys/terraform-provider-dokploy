package project

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	resproject "github.com/vanillauys/terraform-provider-dokploy/internal/resources/project"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ datasource.DataSource                     = (*projectDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*projectDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*projectDataSource)(nil)
)

type projectDataSource struct {
	client *client.Client
}

type dataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	CreatedAt    types.String `tfsdk:"created_at"`
	Environments types.List   `tfsdk:"environments"`
}

func NewDataSource() datasource.DataSource { return &projectDataSource{} }

func (d *projectDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (d *projectDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a Dokploy project by id or by exact name.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Optional: true, Computed: true, Description: "Project id. Set exactly one of `id` or `name`."},
			"name":        schema.StringAttribute{Optional: true, Computed: true, Description: "Exact project name. The lookup errors when zero or many projects match."},
			"description": schema.StringAttribute{Computed: true, Description: "Project description."},
			"created_at":  schema.StringAttribute{Computed: true, Description: "Creation timestamp."},
			"environments": schema.ListNestedAttribute{
				Computed:    true,
				Description: "Environments in this project.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":   schema.StringAttribute{Computed: true, Description: "Environment id."},
						"name": schema.StringAttribute{Computed: true, Description: "Environment name."},
					},
				},
			},
		},
	}
}

func (d *projectDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *projectDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

// findByName does the client-side exact-match filter the spec prescribes
// where the API lacks a lookup query; it errors on multiple matches.
func findByName(projects []client.Project, name string) (*client.Project, error) {
	var found *client.Project
	for i := range projects {
		if projects[i].Name != name {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("multiple projects named %q; look the project up by id instead", name)
		}
		found = &projects[i]
	}
	if found == nil {
		return nil, fmt.Errorf("no project named %q", name)
	}
	return found, nil
}

func (d *projectDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config dataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var p *client.Project
	var err error
	if !config.ID.IsNull() {
		p, err = d.client.GetProject(ctx, config.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Reading project", err.Error())
			return
		}
	} else {
		projects, listErr := d.client.ListProjects(ctx)
		if listErr != nil {
			resp.Diagnostics.AddError("Listing projects", listErr.Error())
			return
		}
		p, err = findByName(projects, config.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Looking up project by name", err.Error())
			return
		}
	}

	config.ID = types.StringValue(p.ProjectID)
	config.Name = types.StringValue(p.Name)
	config.Description = tfutil.StringOrNull(p.Description)
	config.CreatedAt = types.StringValue(p.CreatedAt)
	list, diags := resproject.BuildEnvironments(p.Environments)
	resp.Diagnostics.Append(diags...)
	config.Environments = list
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
