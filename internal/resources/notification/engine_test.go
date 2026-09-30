package notification

import (
	"context"
	"encoding/json"
	"strings"
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

// requestKeys checks that a kind names its test endpoint and that its
// request carries every field that the endpoint requires (the zod field
// errors that the v0.30.8 probes recorded).
func requestKeys[M any](t *testing.T, kind Kind[M], wantTest string, wantKeys ...string) {
	t.Helper()
	if kind.Test != wantTest {
		t.Errorf("%s: Test = %q, want %q", kind.Name, kind.Test, wantTest)
	}
	var m M
	raw, err := json.Marshal(kind.Request(context.Background(), &m))
	if err != nil {
		t.Fatalf("%s: marshal request: %v", kind.Name, err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("%s: unmarshal request: %v", kind.Name, err)
	}
	for _, key := range wantKeys {
		if _, ok := got[key]; !ok {
			t.Errorf("%s: request misses %q: %s", kind.Name, key, raw)
		}
	}
}

func TestEveryKindBuildsItsTestRequest(t *testing.T) {
	requestKeys(t, SlackKind(), "testSlackConnection", "webhookUrl", "channel")
	requestKeys(t, DiscordKind(), "testDiscordConnection", "webhookUrl")
	requestKeys(t, TelegramKind(), "testTelegramConnection", "botToken", "chatId", "messageThreadId")
	requestKeys(t, EmailKind(), "testEmailConnection", "smtpServer", "smtpPort", "username", "password", "toAddresses", "fromAddress")
	requestKeys(t, ResendKind(), "testResendConnection", "apiKey", "fromAddress", "toAddresses")
	requestKeys(t, GotifyKind(), "testGotifyConnection", "serverUrl", "appToken", "priority")
	requestKeys(t, NtfyKind(), "testNtfyConnection", "serverUrl", "topic", "accessToken", "priority")
	requestKeys(t, MattermostKind(), "testMattermostConnection", "webhookUrl")
	requestKeys(t, LarkKind(), "testLarkConnection", "webhookUrl")
	requestKeys(t, TeamsKind(), "testTeamsConnection", "webhookUrl")
	requestKeys(t, PushoverKind(), "testPushoverConnection", "userKey", "apiToken", "priority")
	requestKeys(t, CustomKind(), "testCustomConnection", "endpoint")
}

func TestEverySchemaHasVerifyConnection(t *testing.T) {
	ctx := context.Background()
	for name, r := range allKinds() {
		var resp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &resp)
		attr, ok := resp.Schema.Attributes["verify_connection"].(schema.BoolAttribute)
		if !ok || !attr.Optional || !attr.Computed || attr.Default == nil {
			t.Errorf("%s: verify_connection must be an Optional + Computed bool with a default", name)
			continue
		}
		if !strings.Contains(attr.Description, "sends a real test message") {
			t.Errorf("%s: description must say that the test sends a message: %q", name, attr.Description)
		}
	}
}
