// Package security holds the dokploy_security data source.
package security

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
	Username      types.String `tfsdk:"username"`
}

func flatten(s *client.Security, out *model) {
	out.ID = types.StringValue(s.SecurityID)
	out.ApplicationID = types.StringValue(s.ApplicationID)
	out.Username = types.StringValue(s.Username)
}

// NewDataSource returns the dokploy_security data source.
func NewDataSource() datasource.DataSource {
	child := dsutil.Child{
		Kind: "security",
		What: "a basic-auth record of an application that already exists in Dokploy",
		Example: "data \"dokploy_security\" \"admin\" {\n  application_id = data.dokploy_application.web.id\n" +
			"  username       = \"admin\"\n}",
		Secret: "the password", SecretAttr: "`password`", Resource: "`dokploy_security`",
	}
	return dsutil.NewDataSource(dsutil.Record[model, client.Security]{
		Name:        "security",
		Description: child.Description(),
		Attributes: map[string]schema.Attribute{
			"id":             child.IDAttribute("application_id"),
			"application_id": dsutil.LookupString("Id of the application that owns the record. Set it with `username`."),
			"username":       dsutil.LookupString("Basic-auth user name."),
		},
		Validators: dsutil.ChildValidators("application_id", []string{"username"}, "username"),
		ID:         func(m *model) types.String { return m.ID },
		Get: func(ctx context.Context, c *client.Client, id string) (*client.Security, error) {
			return c.GetSecurity(ctx, id)
		},
		Find: func(ctx context.Context, c *client.Client, m *model) (*client.Security, error) {
			records, err := c.ListSecurities(ctx, m.ApplicationID.ValueString())
			if err != nil {
				return nil, err
			}
			return dsutil.Find(records, func(x client.Security) bool { return x.Username == m.Username.ValueString() },
				"security", "the user name on application "+m.ApplicationID.ValueString())
		},
		Flatten: flatten,
	})()
}
