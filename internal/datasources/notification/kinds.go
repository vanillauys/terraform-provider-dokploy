package notification

import (
	"context"
	"maps"

	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/datasources/dsutil"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

func strOrNull(s string) types.String { return tfutil.StringOrNull(&s) }

func listOf(values []string) types.List {
	list, _ := types.ListValueFrom(context.Background(), types.StringType, values)
	return list
}

// attrs is the attribute map of a channel that has no attribute of its own.
func attrs(entries ...map[string]schema.Attribute) map[string]schema.Attribute {
	out := map[string]schema.Attribute{}
	for _, e := range entries {
		maps.Copy(out, e)
	}
	return out
}

func one(name string, attr schema.Attribute) map[string]schema.Attribute {
	return map[string]schema.Attribute{name: attr}
}

const (
	webhook = "the webhook URL (`webhook_url`)"
	single  = "The attribute"
)

type SlackModel struct {
	Common
	Channel types.String `tfsdk:"channel"`
}

func SlackKind() Kind[SlackModel] {
	return Kind[SlackModel]{
		Name: "slack_notification", Label: "Slack", Type: "slack", Secret: webhook, SecretAttr: single,
		Attributes: one("channel", dsutil.String("Channel that receives the message, or null for the default channel of the webhook.")),
		Common:     func(m *SlackModel) *Common { return &m.Common },
		Flatten:    func(n *client.Notification, m *SlackModel) { m.Channel = strOrNull(n.Slack.Channel) },
	}
}

type DiscordModel struct {
	Common
	Decoration types.Bool `tfsdk:"decoration"`
}

func DiscordKind() Kind[DiscordModel] {
	return Kind[DiscordModel]{
		Name: "discord_notification", Label: "Discord", Type: "discord", Secret: webhook, SecretAttr: single,
		Attributes: one("decoration", dsutil.Bool("Whether the message is a rich embed.")),
		Common:     func(m *DiscordModel) *Common { return &m.Common },
		Flatten:    func(n *client.Notification, m *DiscordModel) { m.Decoration = types.BoolValue(n.Discord.Decoration) },
	}
}

type TelegramModel struct {
	Common
	ChatID          types.String `tfsdk:"chat_id"`
	MessageThreadID types.String `tfsdk:"message_thread_id"`
}

func TelegramKind() Kind[TelegramModel] {
	return Kind[TelegramModel]{
		Name: "telegram_notification", Label: "Telegram", Type: "telegram",
		Secret: "the bot token (`bot_token`)", SecretAttr: single,
		Attributes: attrs(
			one("chat_id", dsutil.String("Id of the chat or group that receives the message.")),
			one("message_thread_id", dsutil.String("Topic id inside a forum group, or null.")),
		),
		Common: func(m *TelegramModel) *Common { return &m.Common },
		Flatten: func(n *client.Notification, m *TelegramModel) {
			m.ChatID = types.StringValue(n.Telegram.ChatID)
			m.MessageThreadID = strOrNull(n.Telegram.MessageThreadID)
		},
	}
}

type EmailModel struct {
	Common
	SMTPServer  types.String `tfsdk:"smtp_server"`
	SMTPPort    types.Int64  `tfsdk:"smtp_port"`
	Username    types.String `tfsdk:"username"`
	FromAddress types.String `tfsdk:"from_address"`
	ToAddresses types.List   `tfsdk:"to_addresses"`
}

func EmailKind() Kind[EmailModel] {
	return Kind[EmailModel]{
		Name: "email_notification", Label: "Email", Type: "email",
		Secret: "the SMTP password (`password`)", SecretAttr: single,
		Attributes: attrs(
			one("smtp_server", dsutil.String("SMTP host name.")),
			one("smtp_port", dsutil.Int64("SMTP port.")),
			one("username", dsutil.String("SMTP login user.")),
			one("from_address", dsutil.String("Sender address.")),
			one("to_addresses", dsutil.StringList("Recipient addresses.")),
		),
		Common: func(m *EmailModel) *Common { return &m.Common },
		Flatten: func(n *client.Notification, m *EmailModel) {
			m.SMTPServer = types.StringValue(n.Email.SMTPServer)
			m.SMTPPort = types.Int64Value(n.Email.SMTPPort)
			m.Username = types.StringValue(n.Email.Username)
			m.FromAddress = types.StringValue(n.Email.FromAddress)
			m.ToAddresses = listOf(n.Email.ToAddresses)
		},
	}
}

type ResendModel struct {
	Common
	FromAddress types.String `tfsdk:"from_address"`
	ToAddresses types.List   `tfsdk:"to_addresses"`
}

func ResendKind() Kind[ResendModel] {
	return Kind[ResendModel]{
		Name: "resend_notification", Label: "Resend", Type: "resend",
		Secret: "the API key (`api_key`)", SecretAttr: single,
		Attributes: attrs(
			one("from_address", dsutil.String("Sender address.")),
			one("to_addresses", dsutil.StringList("Recipient addresses.")),
		),
		Common: func(m *ResendModel) *Common { return &m.Common },
		Flatten: func(n *client.Notification, m *ResendModel) {
			m.FromAddress = types.StringValue(n.Resend.FromAddress)
			m.ToAddresses = listOf(n.Resend.ToAddresses)
		},
	}
}

type GotifyModel struct {
	Common
	ServerURL  types.String `tfsdk:"server_url"`
	Priority   types.Int64  `tfsdk:"priority"`
	Decoration types.Bool   `tfsdk:"decoration"`
}

func GotifyKind() Kind[GotifyModel] {
	return Kind[GotifyModel]{
		Name: "gotify_notification", Label: "Gotify", Type: "gotify",
		Secret: "the app token (`app_token`)", SecretAttr: single,
		Attributes: attrs(
			one("server_url", dsutil.String("URL of the Gotify server.")),
			one("priority", dsutil.Int64("Message priority.")),
			one("decoration", dsutil.Bool("Whether the message has emoji and formatting.")),
		),
		Common: func(m *GotifyModel) *Common { return &m.Common },
		Flatten: func(n *client.Notification, m *GotifyModel) {
			m.ServerURL = types.StringValue(n.Gotify.ServerURL)
			m.Priority = types.Int64Value(n.Gotify.Priority)
			m.Decoration = types.BoolValue(n.Gotify.Decoration)
		},
	}
}

type NtfyModel struct {
	Common
	ServerURL types.String `tfsdk:"server_url"`
	Topic     types.String `tfsdk:"topic"`
	Priority  types.Int64  `tfsdk:"priority"`
}

func NtfyKind() Kind[NtfyModel] {
	return Kind[NtfyModel]{
		Name: "ntfy_notification", Label: "ntfy", Type: "ntfy",
		Secret: "the access token (`access_token`)", SecretAttr: single,
		Attributes: attrs(
			one("server_url", dsutil.String("URL of the ntfy server.")),
			one("topic", dsutil.String("Topic that receives the message.")),
			one("priority", dsutil.Int64("Message priority from 1 (min) to 5 (max).")),
		),
		Common: func(m *NtfyModel) *Common { return &m.Common },
		Flatten: func(n *client.Notification, m *NtfyModel) {
			m.ServerURL = types.StringValue(n.Ntfy.ServerURL)
			m.Topic = types.StringValue(n.Ntfy.Topic)
			m.Priority = types.Int64Value(n.Ntfy.Priority)
		},
	}
}

type MattermostModel struct {
	Common
	Channel  types.String `tfsdk:"channel"`
	Username types.String `tfsdk:"username"`
}

func MattermostKind() Kind[MattermostModel] {
	return Kind[MattermostModel]{
		Name: "mattermost_notification", Label: "Mattermost", Type: "mattermost", Secret: webhook, SecretAttr: single,
		Attributes: attrs(
			one("channel", dsutil.String("Channel that receives the message, or null for the default channel of the webhook.")),
			one("username", dsutil.String("Display name of the poster, or null.")),
		),
		Common: func(m *MattermostModel) *Common { return &m.Common },
		Flatten: func(n *client.Notification, m *MattermostModel) {
			m.Channel = strOrNull(n.Mattermost.Channel)
			m.Username = strOrNull(n.Mattermost.Username)
		},
	}
}

type LarkModel struct{ Common }

func LarkKind() Kind[LarkModel] {
	return Kind[LarkModel]{
		Name: "lark_notification", Label: "Lark", Type: "lark", Secret: webhook, SecretAttr: single,
		Common:  func(m *LarkModel) *Common { return &m.Common },
		Flatten: func(*client.Notification, *LarkModel) {},
	}
}

type TeamsModel struct{ Common }

func TeamsKind() Kind[TeamsModel] {
	return Kind[TeamsModel]{
		Name: "teams_notification", Label: "Microsoft Teams", Type: "teams", Secret: webhook, SecretAttr: single,
		Common:  func(m *TeamsModel) *Common { return &m.Common },
		Flatten: func(*client.Notification, *TeamsModel) {},
	}
}

type PushoverModel struct {
	Common
	Priority types.Int64 `tfsdk:"priority"`
	Retry    types.Int64 `tfsdk:"retry"`
	Expire   types.Int64 `tfsdk:"expire"`
}

func PushoverKind() Kind[PushoverModel] {
	return Kind[PushoverModel]{
		Name: "pushover_notification", Label: "Pushover", Type: "pushover",
		Secret: "the user key and the API token (`user_key`, `api_token`)", SecretAttr: "Each attribute",
		Attributes: attrs(
			one("priority", dsutil.Int64("Message priority from `-2` (lowest) to `2` (emergency).")),
			one("retry", dsutil.Int64("Seconds between repeats of an emergency message, or null.")),
			one("expire", dsutil.Int64("Seconds after which an emergency message stops repeating, or null.")),
		),
		Common: func(m *PushoverModel) *Common { return &m.Common },
		Flatten: func(n *client.Notification, m *PushoverModel) {
			m.Priority = types.Int64Value(n.Pushover.Priority)
			m.Retry = types.Int64PointerValue(n.Pushover.Retry)
			m.Expire = types.Int64PointerValue(n.Pushover.Expire)
		},
	}
}

type CustomModel struct{ Common }

// CustomKind exposes neither the endpoint nor the headers. The endpoint URL
// can hold a token and the headers usually hold a credential.
func CustomKind() Kind[CustomModel] {
	return Kind[CustomModel]{
		Name: "custom_notification", Label: "Custom webhook", Type: "custom",
		Secret: "the endpoint URL and the headers (`endpoint`, `headers`)", SecretAttr: "Each attribute",
		Common:  func(m *CustomModel) *Common { return &m.Common },
		Flatten: func(*client.Notification, *CustomModel) {},
	}
}
