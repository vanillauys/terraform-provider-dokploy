package notification

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// secretAttributes are the attribute names that hold a secret on the
// notification resources. No data source can carry one.
var secretAttributes = []string{
	"webhook_url", "bot_token", "password", "api_key", "app_token", "access_token",
	"user_key", "api_token", "endpoint", "headers",
}

func TestNoSecretReachesADataSource(t *testing.T) {
	constructors := map[string]func() datasource.DataSource{
		"slack_notification":      NewDataSource(SlackKind()),
		"discord_notification":    NewDataSource(DiscordKind()),
		"telegram_notification":   NewDataSource(TelegramKind()),
		"email_notification":      NewDataSource(EmailKind()),
		"resend_notification":     NewDataSource(ResendKind()),
		"gotify_notification":     NewDataSource(GotifyKind()),
		"ntfy_notification":       NewDataSource(NtfyKind()),
		"mattermost_notification": NewDataSource(MattermostKind()),
		"lark_notification":       NewDataSource(LarkKind()),
		"teams_notification":      NewDataSource(TeamsKind()),
		"pushover_notification":   NewDataSource(PushoverKind()),
		"custom_notification":     NewDataSource(CustomKind()),
	}
	for name, newDataSource := range constructors {
		t.Run(name, func(t *testing.T) {
			ds := newDataSource()
			var meta datasource.MetadataResponse
			ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "dokploy"}, &meta)
			if meta.TypeName != "dokploy_"+name {
				t.Errorf("type name = %q", meta.TypeName)
			}
			var resp datasource.SchemaResponse
			ds.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
			if diags := resp.Schema.ValidateImplementation(context.Background()); diags.HasError() {
				t.Fatalf("schema is invalid: %v", diags)
			}
			for _, secret := range secretAttributes {
				if _, found := resp.Schema.Attributes[secret]; found {
					t.Errorf("the data source has the secret attribute %q", secret)
				}
			}
			for _, required := range []string{"id", "name", "created_at", "app_deploy", "server_threshold"} {
				if _, found := resp.Schema.Attributes[required]; !found {
					t.Errorf("the data source lacks the attribute %q", required)
				}
			}
		})
	}
}

func TestFlattenChannelAttributes(t *testing.T) {
	n := &client.Notification{
		NotificationID: "n1", Name: "ops", CreatedAt: "2026-09-30T00:00:00Z",
		NotificationEvents: client.NotificationEvents{AppDeploy: true},
		Pushover:           &client.PushoverNotification{UserKey: "secret", APIToken: "secret", Priority: 2},
		Email:              &client.EmailNotification{SMTPServer: "smtp.example.com", SMTPPort: 587, Username: "u", Password: "secret", FromAddress: "a@example.com", ToAddresses: []string{"b@example.com"}},
	}
	var pushover PushoverModel
	k := PushoverKind()
	k.Common(&pushover).flatten(n)
	k.Flatten(n, &pushover)
	if pushover.ID.ValueString() != "n1" || pushover.Name.ValueString() != "ops" || !pushover.AppDeploy.ValueBool() ||
		pushover.CreatedAt.ValueString() != "2026-09-30T00:00:00Z" || pushover.Priority.ValueInt64() != 2 ||
		!pushover.Retry.IsNull() || !pushover.Expire.IsNull() {
		t.Errorf("pushover model = %+v", pushover)
	}
	var email EmailModel
	e := EmailKind()
	e.Flatten(n, &email)
	if email.SMTPServer.ValueString() != "smtp.example.com" || email.SMTPPort.ValueInt64() != 587 ||
		email.Username.ValueString() != "u" || email.FromAddress.ValueString() != "a@example.com" ||
		len(email.ToAddresses.Elements()) != 1 {
		t.Errorf("email model = %+v", email)
	}
}
