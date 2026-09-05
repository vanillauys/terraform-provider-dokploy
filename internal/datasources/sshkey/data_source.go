// Package sshkey holds the dokploy_ssh_key data source.
package sshkey

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ datasource.DataSource                     = (*sshKeyDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*sshKeyDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*sshKeyDataSource)(nil)
)

type sshKeyDataSource struct{ client *client.Client }

func NewDataSource() datasource.DataSource { return &sshKeyDataSource{} }

// private_key is deliberately NOT modelled: a data source exists to be
// referenced by id, and a copy of the private key in every consumer's state
// widens its exposure for no gain. The dokploy_ssh_key RESOURCE carries it,
// because whoever creates the record has to supply it.
type model struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	PublicKey      types.String `tfsdk:"public_key"`
	OrganizationID types.String `tfsdk:"organization_id"`
	CreatedAt      types.String `tfsdk:"created_at"`
}

func (d *sshKeyDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ssh_key"
}

func (d *sshKeyDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *sshKeyDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up an SSH key that already exists in Dokploy (Settings > SSH Keys), so that a server or a " +
			"private git source can reference it by name:\n\n" +
			"```terraform\n" +
			"data \"dokploy_ssh_key\" \"deploy\" {\n  name = \"deploy\"\n}\n\n" +
			"resource \"dokploy_server\" \"worker\" {\n  ssh_key_id = data.dokploy_ssh_key.deploy.id\n  # ...\n}\n" +
			"```\n\n" +
			"~> **The data source does not expose the private key.** `private_key` exists on the `dokploy_ssh_key` " +
			"resource, but not here, by design. A consumer needs only the id.\n\n" +
			"~> Dokploy does not enforce name uniqueness. If two keys share a name, this data source fails instead of a " +
			"guess. Look the record up by `id` in that case.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "SSH key id. Set it for a lookup by id, or leave it unset and set `name`.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Display name as shown in Dokploy. Set exactly one of `id` or `name`.",
			},
			"description":     schema.StringAttribute{Computed: true, Description: "Free-text description, or null."},
			"public_key":      schema.StringAttribute{Computed: true, Description: "Public key in OpenSSH format."},
			"organization_id": schema.StringAttribute{Computed: true, Description: "Id of the organization that owns the key."},
			"created_at":      schema.StringAttribute{Computed: true, Description: "Creation timestamp from the server."},
		},
	}
}

func (d *sshKeyDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

// findByName resolves a name to exactly one record. It never returns [0] on
// a multiple match: Dokploy does not enforce name uniqueness on SSH keys.
func findByName(keys []client.SSHKey, name string) (*client.SSHKey, error) {
	var matches []client.SSHKey
	for _, k := range keys {
		if k.Name == name {
			matches = append(matches, k)
		}
	}
	switch len(matches) {
	case 1:
		return &matches[0], nil
	case 0:
		return nil, fmt.Errorf("no SSH key named %q", name)
	default:
		return nil, fmt.Errorf(
			"%d SSH keys are named %q; names are not unique in Dokploy, so look it up by id instead",
			len(matches), name)
	}
}

func (d *sshKeyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var found *client.SSHKey
	if id := config.ID.ValueString(); id != "" {
		got, err := d.client.GetSSHKey(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError("Reading the SSH key", err.Error())
			return
		}
		found = got
	} else {
		keys, err := d.client.ListSSHKeys(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Listing SSH keys", err.Error())
			return
		}
		got, err := findByName(keys, config.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Finding the SSH key", err.Error())
			return
		}
		found = got
	}
	config.ID = types.StringValue(found.SSHKeyID)
	config.Name = types.StringValue(found.Name)
	config.Description = tfutil.StringOrNull(&found.Description)
	config.PublicKey = types.StringValue(found.PublicKey)
	config.OrganizationID = types.StringValue(found.OrganizationID)
	config.CreatedAt = types.StringValue(found.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
