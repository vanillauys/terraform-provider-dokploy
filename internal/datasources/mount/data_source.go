// Package mount holds the dokploy_mount data source.
package mount

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/datasources/dsutil"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

type model struct {
	ID          types.String `tfsdk:"id"`
	ServiceID   types.String `tfsdk:"service_id"`
	ServiceType types.String `tfsdk:"service_type"`
	Type        types.String `tfsdk:"type"`
	MountPath   types.String `tfsdk:"mount_path"`
	HostPath    types.String `tfsdk:"host_path"`
	VolumeName  types.String `tfsdk:"volume_name"`
	FilePath    types.String `tfsdk:"file_path"`
	Content     types.String `tfsdk:"content"`
}

// flatten reads the parent from the column that serviceType names, as the
// resource does: a record can carry two parent ids.
func flatten(m *client.Mount, out *model) {
	out.ID = types.StringValue(m.MountID)
	out.ServiceType = types.StringValue(m.ServiceType)
	out.Type = types.StringValue(m.Type)
	out.MountPath = types.StringValue(m.MountPath)
	out.HostPath = tfutil.StringOrNull(m.HostPath)
	out.VolumeName = tfutil.StringOrNull(m.VolumeName)
	out.FilePath = tfutil.StringOrNull(m.FilePath)
	out.Content = tfutil.StringOrNull(m.Content)
	serviceID := m.ServiceID()
	out.ServiceID = tfutil.StringOrNull(&serviceID)
}

// NewDataSource returns the dokploy_mount data source.
func NewDataSource() datasource.DataSource {
	child := dsutil.Child{
		Kind: "mount",
		What: "a volume, bind or file mount of a service that already exists in Dokploy",
		Example: "data \"dokploy_mount\" \"data\" {\n  service_id   = data.dokploy_application.web.id\n" +
			"  service_type = \"application\"\n  mount_path   = \"/data\"\n}",
	}
	return dsutil.NewDataSource(dsutil.Record[model, client.Mount]{
		Name:        "mount",
		Description: child.Description(),
		Attributes: map[string]schema.Attribute{
			"id":           child.IDAttribute("service_id"),
			"service_id":   dsutil.LookupString("Id of the service that owns the mount. Set it with `service_type` and `mount_path` or `host_path`."),
			"service_type": dsutil.LookupString("Type of the service: `application`, `compose`, `postgres`, `mysql`, `mariadb`, `mongo`, `redis` or `libsql`."),
			"mount_path":   dsutil.LookupString("Path inside the container. A lookup can use it as a filter."),
			"host_path":    dsutil.LookupString("Path on the host of a bind mount. A lookup can use it as a filter."),
			"type":         dsutil.String("Mount type: `bind`, `volume` or `file`."),
			"volume_name":  dsutil.String("Volume name of a volume mount, or null."),
			"file_path":    dsutil.String("Path of a file mount, or null."),
			"content":      dsutil.String("Content of a file mount, or null."),
		},
		Validators: dsutil.ChildValidators("service_id", []string{"service_type"}, "mount_path", "host_path"),
		ID:         func(m *model) types.String { return m.ID },
		Get: func(ctx context.Context, c *client.Client, id string) (*client.Mount, error) {
			return c.GetMount(ctx, id)
		},
		Find: func(ctx context.Context, c *client.Client, m *model) (*client.Mount, error) {
			ref := client.ParentRef{Type: m.ServiceType.ValueString(), ID: m.ServiceID.ValueString()}
			mounts, err := c.ListMounts(ctx, ref)
			if err != nil {
				return nil, err
			}
			return dsutil.Find(mounts, func(x client.Mount) bool {
				return (m.MountPath.IsNull() || x.MountPath == m.MountPath.ValueString()) &&
					(m.HostPath.IsNull() || (x.HostPath != nil && *x.HostPath == m.HostPath.ValueString()))
			}, "mount", "the mount path or host path on "+ref.Type+" "+ref.ID)
		},
		Flatten: flatten,
	})()
}
