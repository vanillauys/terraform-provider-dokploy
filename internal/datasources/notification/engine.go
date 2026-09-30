// Package notification holds the twelve dokploy_<channel>_notification data
// sources. They share one engine, as the resources do: every channel has the
// same record (a name, eight event flags, and a channel block), so a Kind
// carries only the channel attributes. No secret reaches a data source. A
// Kind lists the non-secret channel attributes only.
package notification

import (
	"context"
	"fmt"
	"maps"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/datasources/dsutil"
)

// Common holds the attributes that every channel shares. Each channel model
// embeds it by value; the framework promotes the tfsdk fields.
type Common struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	AppDeploy       types.Bool   `tfsdk:"app_deploy"`
	AppBuildError   types.Bool   `tfsdk:"app_build_error"`
	DatabaseBackup  types.Bool   `tfsdk:"database_backup"`
	DockerCleanup   types.Bool   `tfsdk:"docker_cleanup"`
	DokployRestart  types.Bool   `tfsdk:"dokploy_restart"`
	DokployBackup   types.Bool   `tfsdk:"dokploy_backup"`
	VolumeBackup    types.Bool   `tfsdk:"volume_backup"`
	ServerThreshold types.Bool   `tfsdk:"server_threshold"`
	CreatedAt       types.String `tfsdk:"created_at"`
}

func (c *Common) flatten(n *client.Notification) {
	c.ID = types.StringValue(n.NotificationID)
	c.Name = types.StringValue(n.Name)
	c.AppDeploy = types.BoolValue(n.AppDeploy)
	c.AppBuildError = types.BoolValue(n.AppBuildError)
	c.DatabaseBackup = types.BoolValue(n.DatabaseBackup)
	c.DockerCleanup = types.BoolValue(n.DockerCleanup)
	c.DokployRestart = types.BoolValue(n.DokployRestart)
	c.DokployBackup = types.BoolValue(n.DokployBackup)
	c.VolumeBackup = types.BoolValue(n.VolumeBackup)
	c.ServerThreshold = types.BoolValue(n.ServerThreshold)
	c.CreatedAt = types.StringValue(n.CreatedAt)
}

// Kind describes one channel. Per-channel divergence lives here, never in a
// branch of the engine.
type Kind[M any] struct {
	// Name is the type suffix: dokploy_<Name>.
	Name string
	// Label names the channel in messages and descriptions ("Slack").
	Label string
	// Type is the server's notificationType value ("slack").
	Type string
	// Secret names the secret attributes that stay out: "the webhook URL
	// (`webhook_url`)". SecretAttr is the subject of the next sentence.
	Secret, SecretAttr string
	// Attributes are the non-secret channel attributes.
	Attributes map[string]schema.Attribute
	Common     func(*M) *Common
	Flatten    func(*client.Notification, *M)
}

// boolAttributes are the computed event flags of every channel.
var boolAttributes = map[string]string{
	"app_deploy":       "Whether the channel gets a message when an application or a compose deploys.",
	"app_build_error":  "Whether the channel gets a message when a build fails.",
	"database_backup":  "Whether the channel gets a message when a database backup runs.",
	"docker_cleanup":   "Whether the channel gets a message when the Docker cleanup runs.",
	"dokploy_restart":  "Whether the channel gets a message when Dokploy restarts.",
	"dokploy_backup":   "Whether the channel gets a message when the Dokploy server backup runs.",
	"volume_backup":    "Whether the channel gets a message when a volume backup runs.",
	"server_threshold": "Whether the channel gets a message when a server crosses a resource threshold.",
}

// NewDataSource returns the data source constructor for a channel.
func NewDataSource[M any](k Kind[M]) func() datasource.DataSource {
	l := dsutil.Lookup{
		Kind: k.Label + " notification channel", Plural: k.Label + " notification channels",
		What:    "a " + k.Label + " notification channel that already exists in Dokploy (Settings > Notifications)",
		Example: fmt.Sprintf("data \"dokploy_%s\" \"ops\" {\n  name = \"ops\"\n}", k.Name),
		Secret:  k.Secret, SecretAttr: k.SecretAttr, Resource: "`dokploy_" + k.Name + "`",
	}
	attributes := l.Attributes()
	for name, description := range boolAttributes {
		attributes[name] = dsutil.Bool(description)
	}
	attributes["created_at"] = dsutil.String("Creation timestamp from the server.")
	maps.Copy(attributes, k.Attributes)
	return dsutil.NewDataSource(dsutil.Record[M, client.Notification]{
		Name:        k.Name,
		Description: l.Description(),
		Attributes:  attributes,
		Validators:  dsutil.IDOrName(),
		ID:          func(m *M) types.String { return k.Common(m).ID },
		Get: func(ctx context.Context, c *client.Client, id string) (*client.Notification, error) {
			n, err := c.GetNotification(ctx, id)
			if err != nil {
				return nil, err
			}
			if n.NotificationType != k.Type {
				return nil, fmt.Errorf("notification %s is a %s channel, not %s; use the matching data source", id, n.NotificationType, k.Type)
			}
			return n, nil
		},
		Find: func(ctx context.Context, c *client.Client, m *M) (*client.Notification, error) {
			name := k.Common(m).Name.ValueString()
			all, err := c.ListNotifications(ctx)
			if err != nil {
				return nil, err
			}
			return dsutil.Find(all, func(n client.Notification) bool {
				return n.NotificationType == k.Type && n.Name == name
			}, k.Label+" notification channel", "the name "+name)
		},
		Flatten: func(n *client.Notification, m *M) {
			k.Common(m).flatten(n)
			k.Flatten(n, m)
		},
	})
}
