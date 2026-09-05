package notification

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// allKinds builds every channel resource once, so one loop can validate
// the twelve schemas the framework way.
func allKinds() map[string]resource.Resource {
	return map[string]resource.Resource{
		"slack":      NewResource(SlackKind())(),
		"discord":    NewResource(DiscordKind())(),
		"telegram":   NewResource(TelegramKind())(),
		"email":      NewResource(EmailKind())(),
		"resend":     NewResource(ResendKind())(),
		"gotify":     NewResource(GotifyKind())(),
		"ntfy":       NewResource(NtfyKind())(),
		"mattermost": NewResource(MattermostKind())(),
		"lark":       NewResource(LarkKind())(),
		"teams":      NewResource(TeamsKind())(),
		"pushover":   NewResource(PushoverKind())(),
		"custom":     NewResource(CustomKind())(),
	}
}

func TestEverySchemaValidates(t *testing.T) {
	ctx := context.Background()
	for name, r := range allKinds() {
		var resp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("%s Schema(): %v", name, resp.Diagnostics)
		}
		if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
			t.Errorf("%s ValidateImplementation(): %v", name, diags)
		}
		for _, attr := range []string{"id", "name", "app_deploy", "server_threshold", "created_at"} {
			if _, ok := resp.Schema.Attributes[attr]; !ok {
				t.Errorf("%s: shared attribute %q missing", name, attr)
			}
		}
		var meta resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "dokploy"}, &meta)
		if meta.TypeName != "dokploy_"+name+"_notification" {
			t.Errorf("%s: type name = %q", name, meta.TypeName)
		}
	}
}

// TestSecretsCarryCompanions pins the write-only pair on every secret of
// every kind, and the conflict-only shape on the one optional secret.
func TestSecretsCarryCompanions(t *testing.T) {
	ctx := context.Background()
	for name, r := range allKinds() {
		var resp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &resp)
		for attr, a := range resp.Schema.Attributes {
			s, ok := a.(schema.StringAttribute)
			if !ok || !s.Sensitive || s.WriteOnly {
				continue
			}
			wo, ok := resp.Schema.Attributes[attr+"_wo"].(schema.StringAttribute)
			if !ok || !wo.WriteOnly || !wo.Sensitive {
				t.Errorf("%s: %s has no write-only companion", name, attr)
			}
			if _, ok := resp.Schema.Attributes[attr+"_wo_version"].(schema.Int64Attribute); !ok {
				t.Errorf("%s: %s has no version companion", name, attr)
			}
		}
	}
}

func TestCommonFlattenAndBase(t *testing.T) {
	var c Common
	c.flatten(&client.Notification{NotificationID: "n1", Name: "ops", CreatedAt: "t",
		NotificationEvents: client.NotificationEvents{AppDeploy: true, ServerThreshold: true}})
	if c.ID.ValueString() != "n1" || c.Name.ValueString() != "ops" || !c.AppDeploy.ValueBool() || c.AppBuildError.ValueBool() ||
		!c.ServerThreshold.ValueBool() || c.CreatedAt.ValueString() != "t" {
		t.Errorf("flatten() = %+v", c)
	}
	b := c.base()
	if b.Name != "ops" || !b.AppDeploy || b.DokployBackup || !b.ServerThreshold {
		t.Errorf("base() = %+v", b)
	}
}

func TestCustomFlattenCollapsesEmptyHeaders(t *testing.T) {
	var m CustomModel
	CustomKind().Flatten(&client.Notification{Custom: &client.CustomNotification{Endpoint: "https://x"}}, &m)
	if !m.Headers.IsNull() || m.Endpoint.ValueString() != "https://x" {
		t.Errorf("flatten() = %+v", m)
	}
	CustomKind().Flatten(&client.Notification{Custom: &client.CustomNotification{Headers: map[string]string{"A": "b"}}}, &m)
	if m.Headers.IsNull() || len(m.Headers.Elements()) != 1 {
		t.Errorf("flatten() headers = %v", m.Headers)
	}
	if got := headersOf(context.Background(), types.MapNull(types.StringType)); got == nil || len(got) != 0 {
		t.Errorf("headersOf(null) = %v, want an empty (non-nil) map, which the server accepts as no headers", got)
	}
}
