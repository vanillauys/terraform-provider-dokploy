// Package destination holds the dokploy_destination data source.
//
// destination is the only child-free record besides project, environment and
// the git providers with a natural key: `name` is a display name on a
// top-level record, not scoped to a parent. The backup plane's other records
// (backup, volume_backup) carry a name scoped to a parent service, and
// mount/port/redirect/security have no name at all - a data source over any
// of those would be ambiguous by construction, which is exactly what the
// never-take-[0] rule exists to prevent.
package destination

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
	_ datasource.DataSource                     = (*destinationDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*destinationDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*destinationDataSource)(nil)
)

type destinationDataSource struct{ client *client.Client }

func NewDataSource() datasource.DataSource { return &destinationDataSource{} }

// secret_access_key and access_key are deliberately NOT modelled. A data
// source exists to be referenced in configuration, and putting credentials
// for a shared backup target into every consumer's state widens their blast
// radius for no gain: dokploy_backup and dokploy_volume_backup need only the
// destination's id. The dokploy_destination RESOURCE still carries both,
// because whoever creates the record has to supply them.
type model struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	ProviderName    types.String `tfsdk:"provider_name"`
	Endpoint        types.String `tfsdk:"endpoint"`
	Bucket          types.String `tfsdk:"bucket"`
	Region          types.String `tfsdk:"region"`
	AdditionalFlags types.List   `tfsdk:"additional_flags"`
	CreatedAt       types.String `tfsdk:"created_at"`
}

func (d *destinationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_destination"
}

func (d *destinationDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *destinationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up an S3-compatible backup destination already registered in Dokploy.\n\n" +
			"A shared backup target is typically created once and referenced from several projects:\n\n" +
			"```terraform\n" +
			"data \"dokploy_destination\" \"backups\" {\n  name = \"cloudflare-r2\"\n}\n\n" +
			"resource \"dokploy_backup\" \"db\" {\n  destination_id = data.dokploy_destination.backups.id\n  # ...\n}\n" +
			"```\n\n" +
			"~> **Credentials are not exposed.** `access_key` and `secret_access_key` exist on the " +
			"`dokploy_destination` resource but deliberately not here: consumers need only the id, and " +
			"copying an access key into every consumer's state widens its blast radius for no gain.\n\n" +
			"~> Dokploy does not enforce name uniqueness. If two destinations share a name this data " +
			"source fails rather than picking one; look the record up by `id` in that case.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Destination id. Set this to look it up by id, or leave it unset and set `name`.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Display name as shown in Dokploy. Exactly one of `id` or `name` must be set.",
			},
			"provider_name": schema.StringAttribute{
				Computed: true,
				// Named provider_name, not provider, for the same reason the
				// resource is: `provider` is a reserved meta-argument in
				// Terraform configuration and cannot be an attribute name.
				Description: "Storage provider label, e.g. `Cloudflare`, `AWS`, `DigitalOcean`.",
			},
			"endpoint": schema.StringAttribute{Computed: true, Description: "S3 endpoint URL."},
			"bucket":   schema.StringAttribute{Computed: true, Description: "Bucket name."},
			"region":   schema.StringAttribute{Computed: true, Description: "Bucket region."},
			"additional_flags": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "Extra flags passed to the underlying storage client. Empty when none are set.",
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "Creation timestamp (server-side).",
			},
		},
	}
}

func (d *destinationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

// findByName resolves a destination name to exactly one record.
//
// It never returns dests[0] on a multiple match. Dokploy does not enforce
// name uniqueness on destinations, so two records may legitimately share a
// name, and silently picking the first would bind configuration to whichever
// order the server happened to return - a data source that resolves
// differently between plans with no visible cause.
func findByName(dests []client.Destination, name string) (*client.Destination, error) {
	var matches []client.Destination
	for _, d := range dests {
		if d.Name == name {
			matches = append(matches, d)
		}
	}
	switch len(matches) {
	case 1:
		return &matches[0], nil
	case 0:
		return nil, fmt.Errorf("no destination named %q", name)
	default:
		return nil, fmt.Errorf(
			"%d destinations are named %q; names are not unique in Dokploy, so look it up by id instead",
			len(matches), name)
	}
}

func (d *destinationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// ExactlyOneOf above guarantees exactly one of these is set. The id path
	// reads the record directly rather than filtering destination.all: it is
	// one request instead of one plus a scan, and a wrong id surfaces as the
	// server's own not-found rather than as "no destination named".
	var found *client.Destination
	if id := config.ID.ValueString(); id != "" {
		got, err := d.client.GetDestination(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError("Reading the destination", err.Error())
			return
		}
		found = got
	} else {
		dests, err := d.client.ListDestinations(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Listing destinations", err.Error())
			return
		}
		got, err := findByName(dests, config.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Finding the destination", err.Error())
			return
		}
		found = got
	}

	config.ID = types.StringValue(found.DestinationID)
	config.Name = types.StringValue(found.Name)
	config.ProviderName = types.StringValue(found.Provider)
	config.Endpoint = types.StringValue(found.Endpoint)
	config.Bucket = types.StringValue(found.Bucket)
	config.Region = types.StringValue(found.Region)
	config.CreatedAt = types.StringValue(found.CreatedAt)

	flags, diags := types.ListValueFrom(ctx, types.StringType, found.AdditionalFlags)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	config.AdditionalFlags = flags

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
