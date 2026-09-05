// Package user holds the dokploy_user data source.
package user

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
	_ datasource.DataSource                     = (*userDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*userDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*userDataSource)(nil)
)

type userDataSource struct{ client *client.Client }

func NewDataSource() datasource.DataSource { return &userDataSource{} }

type model struct {
	ID           types.String `tfsdk:"id"`
	Email        types.String `tfsdk:"email"`
	MemberID     types.String `tfsdk:"member_id"`
	Role         types.String `tfsdk:"role"`
	FirstName    types.String `tfsdk:"first_name"`
	LastName     types.String `tfsdk:"last_name"`
	IsRegistered types.Bool   `tfsdk:"is_registered"`
	CreatedAt    types.String `tfsdk:"created_at"`
}

func (d *userDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (d *userDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("email")),
	}
}

func (d *userDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up a member of the active organization by user id or by email, for example a person who " +
			"was invited in the Dokploy UI, so that `dokploy_user_permissions` can manage their permissions:\n\n" +
			"```terraform\n" +
			"data \"dokploy_user\" \"dev\" {\n  email = \"dev@example.com\"\n}\n\n" +
			"resource \"dokploy_user_permissions\" \"dev\" {\n  user_id             = data.dokploy_user.dev.id\n  can_create_services = true\n}\n" +
			"```",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional: true, Computed: true,
				Description: "User id. Set it for a lookup by id, or leave it unset and set `email`.",
			},
			"email": schema.StringAttribute{
				Optional: true, Computed: true,
				Description: "Login email. Set exactly one of `id` or `email`.",
			},
			"member_id":     schema.StringAttribute{Computed: true, Description: "Id of the membership record in the active organization."},
			"role":          schema.StringAttribute{Computed: true, Description: "Member role: `owner`, `admin`, `member`, or a custom role."},
			"first_name":    schema.StringAttribute{Computed: true, Description: "First name, or null."},
			"last_name":     schema.StringAttribute{Computed: true, Description: "Last name, or null."},
			"is_registered": schema.BoolAttribute{Computed: true, Description: "Whether the person completed the sign-up."},
			"created_at":    schema.StringAttribute{Computed: true, Description: "Creation timestamp of the membership from the server."},
		},
	}
}

func (d *userDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func findByEmail(members []client.Member, email string) (*client.Member, error) {
	for i := range members {
		if members[i].User.Email == email {
			return &members[i], nil
		}
	}
	return nil, fmt.Errorf("no member with email %q in the active organization", email)
}

func (d *userDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var found *client.Member
	if id := config.ID.ValueString(); id != "" {
		got, err := d.client.GetMember(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError("Reading the user", err.Error())
			return
		}
		found = got
	} else {
		members, err := d.client.ListMembers(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Listing members", err.Error())
			return
		}
		got, err := findByEmail(members, config.Email.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Finding the user", err.Error())
			return
		}
		found = got
	}
	config.ID = types.StringValue(found.UserID)
	config.Email = types.StringValue(found.User.Email)
	config.MemberID = types.StringValue(found.ID)
	config.Role = types.StringValue(found.Role)
	config.FirstName = tfutil.StringOrNull(&found.User.FirstName)
	config.LastName = tfutil.StringOrNull(&found.User.LastName)
	config.IsRegistered = types.BoolValue(found.User.IsRegistered)
	config.CreatedAt = types.StringValue(found.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
