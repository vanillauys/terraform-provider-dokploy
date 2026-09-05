package notification

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

func strOrNull(s string) types.String { return tfutil.StringOrNull(&s) }

// secretAttribute is the plain half of a write-only pair: Optional only
// because the companion can replace it.
func secretAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{Optional: true, Sensitive: true, Description: description}
}

func stringsOf(ctx context.Context, list types.List) []string {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}
	var out []string
	_ = list.ElementsAs(ctx, &out, false)
	return out
}

func listOf(ctx context.Context, values []string) types.List {
	list, _ := types.ListValueFrom(ctx, types.StringType, values)
	return list
}

// ---------------------------------------------------------------- slack

type SlackModel struct {
	Common
	WebhookURL          types.String `tfsdk:"webhook_url"`
	WebhookURLWo        types.String `tfsdk:"webhook_url_wo"`
	WebhookURLWoVersion types.Int64  `tfsdk:"webhook_url_wo_version"`
	Channel             types.String `tfsdk:"channel"`
}

func SlackKind() Kind[SlackModel] {
	return Kind[SlackModel]{
		Name: "slack_notification", Label: "Slack", Type: "slack",
		Intro: "A Slack notification channel (Settings > Notifications). Dokploy posts each message to an incoming webhook.",
		Attributes: map[string]schema.Attribute{
			"webhook_url": secretAttribute("Incoming webhook URL of the Slack app. Set this attribute or `webhook_url_wo`."),
			"channel": schema.StringAttribute{
				Optional: true,
				Description: "Channel that receives the message, for example `#deploys`. Omit it to use the webhook's " +
					"default channel. If you remove it from the configuration, the provider clears it.",
			},
		},
		Common: func(m *SlackModel) *Common { return &m.Common },
		Create: func(ctx context.Context, c *client.Client, m *SlackModel) (*client.Notification, error) {
			return c.CreateSlackNotification(ctx, client.CreateSlackNotificationRequest{
				NotificationBase: m.base(), WebhookURL: m.WebhookURL.ValueString(), Channel: m.Channel.ValueString(),
			})
		},
		Update: func(ctx context.Context, c *client.Client, m *SlackModel, n *client.Notification) error {
			return c.UpdateSlackNotification(ctx, client.UpdateSlackNotificationRequest{
				NotificationID: m.ID.ValueString(), SlackID: n.Slack.SlackID,
				NotificationBase: m.base(), WebhookURL: m.WebhookURL.ValueString(), Channel: m.Channel.ValueString(),
			})
		},
		Flatten: func(n *client.Notification, m *SlackModel) {
			m.WebhookURL = types.StringValue(n.Slack.WebhookURL)
			m.Channel = strOrNull(n.Slack.Channel)
		},
		Secrets: []secret[SlackModel]{{
			name:    "webhook_url",
			plain:   func(m *SlackModel) *types.String { return &m.WebhookURL },
			wo:      func(m *SlackModel) *types.String { return &m.WebhookURLWo },
			version: func(m *SlackModel) *types.Int64 { return &m.WebhookURLWoVersion },
			stored:  func(n *client.Notification) string { return n.Slack.WebhookURL },
		}},
	}
}

// ---------------------------------------------------------------- discord

type DiscordModel struct {
	Common
	WebhookURL          types.String `tfsdk:"webhook_url"`
	WebhookURLWo        types.String `tfsdk:"webhook_url_wo"`
	WebhookURLWoVersion types.Int64  `tfsdk:"webhook_url_wo_version"`
	Decoration          types.Bool   `tfsdk:"decoration"`
}

func DiscordKind() Kind[DiscordModel] {
	return Kind[DiscordModel]{
		Name: "discord_notification", Label: "Discord", Type: "discord",
		Intro: "A Discord notification channel (Settings > Notifications). Dokploy posts each message to a channel webhook.",
		Attributes: map[string]schema.Attribute{
			"webhook_url": secretAttribute("Webhook URL of the Discord channel. Set this attribute or `webhook_url_wo`."),
			"decoration": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Send the message as a rich embed with colors and the Dokploy icon. Defaults to `true`.",
			},
		},
		Common: func(m *DiscordModel) *Common { return &m.Common },
		Create: func(ctx context.Context, c *client.Client, m *DiscordModel) (*client.Notification, error) {
			return c.CreateDiscordNotification(ctx, client.CreateDiscordNotificationRequest{
				NotificationBase: m.base(), WebhookURL: m.WebhookURL.ValueString(), Decoration: m.Decoration.ValueBool(),
			})
		},
		Update: func(ctx context.Context, c *client.Client, m *DiscordModel, n *client.Notification) error {
			return c.UpdateDiscordNotification(ctx, client.UpdateDiscordNotificationRequest{
				NotificationID: m.ID.ValueString(), DiscordID: n.Discord.DiscordID,
				NotificationBase: m.base(), WebhookURL: m.WebhookURL.ValueString(), Decoration: m.Decoration.ValueBool(),
			})
		},
		Flatten: func(n *client.Notification, m *DiscordModel) {
			m.WebhookURL = types.StringValue(n.Discord.WebhookURL)
			m.Decoration = types.BoolValue(n.Discord.Decoration)
		},
		Secrets: []secret[DiscordModel]{{
			name:    "webhook_url",
			plain:   func(m *DiscordModel) *types.String { return &m.WebhookURL },
			wo:      func(m *DiscordModel) *types.String { return &m.WebhookURLWo },
			version: func(m *DiscordModel) *types.Int64 { return &m.WebhookURLWoVersion },
			stored:  func(n *client.Notification) string { return n.Discord.WebhookURL },
		}},
	}
}

// ---------------------------------------------------------------- telegram

type TelegramModel struct {
	Common
	BotToken          types.String `tfsdk:"bot_token"`
	BotTokenWo        types.String `tfsdk:"bot_token_wo"`
	BotTokenWoVersion types.Int64  `tfsdk:"bot_token_wo_version"`
	ChatID            types.String `tfsdk:"chat_id"`
	MessageThreadID   types.String `tfsdk:"message_thread_id"`
}

func TelegramKind() Kind[TelegramModel] {
	return Kind[TelegramModel]{
		Name: "telegram_notification", Label: "Telegram", Type: "telegram",
		Intro: "A Telegram notification channel (Settings > Notifications). Dokploy sends each message through a bot to a chat.",
		Attributes: map[string]schema.Attribute{
			"bot_token": secretAttribute("Token of the bot, from BotFather. Set this attribute or `bot_token_wo`."),
			"chat_id":   schema.StringAttribute{Required: true, Description: "Id of the chat or group that receives the message."},
			"message_thread_id": schema.StringAttribute{
				Optional: true,
				Description: "Topic id inside a forum group. Omit it for a plain chat. If you remove it from the " +
					"configuration, the provider clears it.",
			},
		},
		Common: func(m *TelegramModel) *Common { return &m.Common },
		Create: func(ctx context.Context, c *client.Client, m *TelegramModel) (*client.Notification, error) {
			return c.CreateTelegramNotification(ctx, client.CreateTelegramNotificationRequest{
				NotificationBase: m.base(), BotToken: m.BotToken.ValueString(), ChatID: m.ChatID.ValueString(),
				MessageThreadID: m.MessageThreadID.ValueString(),
			})
		},
		Update: func(ctx context.Context, c *client.Client, m *TelegramModel, n *client.Notification) error {
			return c.UpdateTelegramNotification(ctx, client.UpdateTelegramNotificationRequest{
				NotificationID: m.ID.ValueString(), TelegramID: n.Telegram.TelegramID,
				NotificationBase: m.base(), BotToken: m.BotToken.ValueString(), ChatID: m.ChatID.ValueString(),
				MessageThreadID: m.MessageThreadID.ValueString(),
			})
		},
		Flatten: func(n *client.Notification, m *TelegramModel) {
			m.BotToken = types.StringValue(n.Telegram.BotToken)
			m.ChatID = types.StringValue(n.Telegram.ChatID)
			m.MessageThreadID = strOrNull(n.Telegram.MessageThreadID)
		},
		Secrets: []secret[TelegramModel]{{
			name:    "bot_token",
			plain:   func(m *TelegramModel) *types.String { return &m.BotToken },
			wo:      func(m *TelegramModel) *types.String { return &m.BotTokenWo },
			version: func(m *TelegramModel) *types.Int64 { return &m.BotTokenWoVersion },
			stored:  func(n *client.Notification) string { return n.Telegram.BotToken },
		}},
	}
}

// ---------------------------------------------------------------- email

type EmailModel struct {
	Common
	SMTPServer        types.String `tfsdk:"smtp_server"`
	SMTPPort          types.Int64  `tfsdk:"smtp_port"`
	Username          types.String `tfsdk:"username"`
	Password          types.String `tfsdk:"password"`
	PasswordWo        types.String `tfsdk:"password_wo"`
	PasswordWoVersion types.Int64  `tfsdk:"password_wo_version"`
	FromAddress       types.String `tfsdk:"from_address"`
	ToAddresses       types.List   `tfsdk:"to_addresses"`
}

func EmailKind() Kind[EmailModel] {
	return Kind[EmailModel]{
		Name: "email_notification", Label: "Email", Type: "email",
		Intro: "An email notification channel (Settings > Notifications). Dokploy sends each message through an SMTP server.",
		Attributes: map[string]schema.Attribute{
			"smtp_server": schema.StringAttribute{Required: true, Description: "SMTP host name."},
			"smtp_port": schema.Int64Attribute{
				Required: true, Description: "SMTP port, for example `587` for STARTTLS or `465` for TLS.",
				Validators: []validator.Int64{int64validator.Between(1, 65535)},
			},
			"username":     schema.StringAttribute{Required: true, Description: "SMTP login user."},
			"password":     secretAttribute("SMTP login password. Set this attribute or `password_wo`."),
			"from_address": schema.StringAttribute{Required: true, Description: "Sender address."},
			"to_addresses": schema.ListAttribute{
				Required: true, ElementType: types.StringType,
				Description: "Recipient addresses. At least one.",
				Validators:  []validator.List{listvalidator.SizeAtLeast(1)},
			},
		},
		Common: func(m *EmailModel) *Common { return &m.Common },
		Create: func(ctx context.Context, c *client.Client, m *EmailModel) (*client.Notification, error) {
			return c.CreateEmailNotification(ctx, client.CreateEmailNotificationRequest{
				NotificationBase: m.base(), SMTPServer: m.SMTPServer.ValueString(), SMTPPort: m.SMTPPort.ValueInt64(),
				Username: m.Username.ValueString(), Password: m.Password.ValueString(),
				FromAddress: m.FromAddress.ValueString(), ToAddresses: stringsOf(ctx, m.ToAddresses),
			})
		},
		Update: func(ctx context.Context, c *client.Client, m *EmailModel, n *client.Notification) error {
			return c.UpdateEmailNotification(ctx, client.UpdateEmailNotificationRequest{
				NotificationID: m.ID.ValueString(), EmailID: n.Email.EmailID,
				NotificationBase: m.base(), SMTPServer: m.SMTPServer.ValueString(), SMTPPort: m.SMTPPort.ValueInt64(),
				Username: m.Username.ValueString(), Password: m.Password.ValueString(),
				FromAddress: m.FromAddress.ValueString(), ToAddresses: stringsOf(ctx, m.ToAddresses),
			})
		},
		Flatten: func(n *client.Notification, m *EmailModel) {
			m.SMTPServer = types.StringValue(n.Email.SMTPServer)
			m.SMTPPort = types.Int64Value(n.Email.SMTPPort)
			m.Username = types.StringValue(n.Email.Username)
			m.Password = types.StringValue(n.Email.Password)
			m.FromAddress = types.StringValue(n.Email.FromAddress)
			m.ToAddresses = listOf(context.Background(), n.Email.ToAddresses)
		},
		Secrets: []secret[EmailModel]{{
			name:    "password",
			plain:   func(m *EmailModel) *types.String { return &m.Password },
			wo:      func(m *EmailModel) *types.String { return &m.PasswordWo },
			version: func(m *EmailModel) *types.Int64 { return &m.PasswordWoVersion },
			stored:  func(n *client.Notification) string { return n.Email.Password },
		}},
	}
}

// ---------------------------------------------------------------- resend

type ResendModel struct {
	Common
	APIKey          types.String `tfsdk:"api_key"`
	APIKeyWo        types.String `tfsdk:"api_key_wo"`
	APIKeyWoVersion types.Int64  `tfsdk:"api_key_wo_version"`
	FromAddress     types.String `tfsdk:"from_address"`
	ToAddresses     types.List   `tfsdk:"to_addresses"`
}

func ResendKind() Kind[ResendModel] {
	return Kind[ResendModel]{
		Name: "resend_notification", Label: "Resend", Type: "resend",
		Intro: "A Resend notification channel (Settings > Notifications). Dokploy sends each message as an email through the Resend API.",
		Attributes: map[string]schema.Attribute{
			"api_key":      secretAttribute("Resend API key. Set this attribute or `api_key_wo`."),
			"from_address": schema.StringAttribute{Required: true, Description: "Sender address on a domain that Resend verified."},
			"to_addresses": schema.ListAttribute{
				Required: true, ElementType: types.StringType,
				Description: "Recipient addresses. At least one.",
				Validators:  []validator.List{listvalidator.SizeAtLeast(1)},
			},
		},
		Common: func(m *ResendModel) *Common { return &m.Common },
		Create: func(ctx context.Context, c *client.Client, m *ResendModel) (*client.Notification, error) {
			return c.CreateResendNotification(ctx, client.CreateResendNotificationRequest{
				NotificationBase: m.base(), APIKey: m.APIKey.ValueString(),
				FromAddress: m.FromAddress.ValueString(), ToAddresses: stringsOf(ctx, m.ToAddresses),
			})
		},
		Update: func(ctx context.Context, c *client.Client, m *ResendModel, n *client.Notification) error {
			return c.UpdateResendNotification(ctx, client.UpdateResendNotificationRequest{
				NotificationID: m.ID.ValueString(), ResendID: n.Resend.ResendID,
				NotificationBase: m.base(), APIKey: m.APIKey.ValueString(),
				FromAddress: m.FromAddress.ValueString(), ToAddresses: stringsOf(ctx, m.ToAddresses),
			})
		},
		Flatten: func(n *client.Notification, m *ResendModel) {
			m.APIKey = types.StringValue(n.Resend.APIKey)
			m.FromAddress = types.StringValue(n.Resend.FromAddress)
			m.ToAddresses = listOf(context.Background(), n.Resend.ToAddresses)
		},
		Secrets: []secret[ResendModel]{{
			name:    "api_key",
			plain:   func(m *ResendModel) *types.String { return &m.APIKey },
			wo:      func(m *ResendModel) *types.String { return &m.APIKeyWo },
			version: func(m *ResendModel) *types.Int64 { return &m.APIKeyWoVersion },
			stored:  func(n *client.Notification) string { return n.Resend.APIKey },
		}},
	}
}

// ---------------------------------------------------------------- gotify

type GotifyModel struct {
	Common
	ServerURL         types.String `tfsdk:"server_url"`
	AppToken          types.String `tfsdk:"app_token"`
	AppTokenWo        types.String `tfsdk:"app_token_wo"`
	AppTokenWoVersion types.Int64  `tfsdk:"app_token_wo_version"`
	Priority          types.Int64  `tfsdk:"priority"`
	Decoration        types.Bool   `tfsdk:"decoration"`
}

func GotifyKind() Kind[GotifyModel] {
	return Kind[GotifyModel]{
		Name: "gotify_notification", Label: "Gotify", Type: "gotify",
		Intro: "A Gotify notification channel (Settings > Notifications). Dokploy pushes each message to a Gotify server.",
		Attributes: map[string]schema.Attribute{
			"server_url": schema.StringAttribute{Required: true, Description: "URL of the Gotify server."},
			"app_token":  secretAttribute("Token of the Gotify application. Set this attribute or `app_token_wo`."),
			"priority": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(5),
				Description: "Message priority, 1 or higher. Defaults to `5`.",
				Validators:  []validator.Int64{int64validator.AtLeast(1)},
			},
			"decoration": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Include emoji and formatting in the message. Defaults to `true`.",
			},
		},
		Common: func(m *GotifyModel) *Common { return &m.Common },
		Create: func(ctx context.Context, c *client.Client, m *GotifyModel) (*client.Notification, error) {
			return c.CreateGotifyNotification(ctx, client.CreateGotifyNotificationRequest{
				NotificationBase: m.base(), ServerURL: m.ServerURL.ValueString(), AppToken: m.AppToken.ValueString(),
				Priority: m.Priority.ValueInt64(), Decoration: m.Decoration.ValueBool(),
			})
		},
		Update: func(ctx context.Context, c *client.Client, m *GotifyModel, n *client.Notification) error {
			return c.UpdateGotifyNotification(ctx, client.UpdateGotifyNotificationRequest{
				NotificationID: m.ID.ValueString(), GotifyID: n.Gotify.GotifyID,
				NotificationBase: m.base(), ServerURL: m.ServerURL.ValueString(), AppToken: m.AppToken.ValueString(),
				Priority: m.Priority.ValueInt64(), Decoration: m.Decoration.ValueBool(),
			})
		},
		Flatten: func(n *client.Notification, m *GotifyModel) {
			m.ServerURL = types.StringValue(n.Gotify.ServerURL)
			m.AppToken = types.StringValue(n.Gotify.AppToken)
			m.Priority = types.Int64Value(n.Gotify.Priority)
			m.Decoration = types.BoolValue(n.Gotify.Decoration)
		},
		Secrets: []secret[GotifyModel]{{
			name:    "app_token",
			plain:   func(m *GotifyModel) *types.String { return &m.AppToken },
			wo:      func(m *GotifyModel) *types.String { return &m.AppTokenWo },
			version: func(m *GotifyModel) *types.Int64 { return &m.AppTokenWoVersion },
			stored:  func(n *client.Notification) string { return n.Gotify.AppToken },
		}},
	}
}

// ---------------------------------------------------------------- ntfy

type NtfyModel struct {
	Common
	ServerURL            types.String `tfsdk:"server_url"`
	Topic                types.String `tfsdk:"topic"`
	AccessToken          types.String `tfsdk:"access_token"`
	AccessTokenWo        types.String `tfsdk:"access_token_wo"`
	AccessTokenWoVersion types.Int64  `tfsdk:"access_token_wo_version"`
	Priority             types.Int64  `tfsdk:"priority"`
}

func NtfyKind() Kind[NtfyModel] {
	return Kind[NtfyModel]{
		Name: "ntfy_notification", Label: "ntfy", Type: "ntfy",
		Intro: "An ntfy notification channel (Settings > Notifications). Dokploy publishes each message to a topic on an ntfy server.",
		Attributes: map[string]schema.Attribute{
			"server_url": schema.StringAttribute{Required: true, Description: "URL of the ntfy server, for example `https://ntfy.sh`."},
			"topic":      schema.StringAttribute{Required: true, Description: "Topic that receives the message."},
			"access_token": secretAttribute("Access token for a protected topic. Omit it for a public topic. Do not set it " +
				"together with `access_token_wo`. If you remove both from the configuration, the provider clears it."),
			"priority": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(3),
				Description: "Message priority from 1 (min) to 5 (max). Defaults to `3`.",
				Validators:  []validator.Int64{int64validator.Between(1, 5)},
			},
		},
		Common: func(m *NtfyModel) *Common { return &m.Common },
		Create: func(ctx context.Context, c *client.Client, m *NtfyModel) (*client.Notification, error) {
			return c.CreateNtfyNotification(ctx, client.CreateNtfyNotificationRequest{
				NotificationBase: m.base(), ServerURL: m.ServerURL.ValueString(), Topic: m.Topic.ValueString(),
				AccessToken: m.AccessToken.ValueString(), Priority: m.Priority.ValueInt64(),
			})
		},
		Update: func(ctx context.Context, c *client.Client, m *NtfyModel, n *client.Notification) error {
			return c.UpdateNtfyNotification(ctx, client.UpdateNtfyNotificationRequest{
				NotificationID: m.ID.ValueString(), NtfyID: n.Ntfy.NtfyID,
				NotificationBase: m.base(), ServerURL: m.ServerURL.ValueString(), Topic: m.Topic.ValueString(),
				AccessToken: m.AccessToken.ValueString(), Priority: m.Priority.ValueInt64(),
			})
		},
		Flatten: func(n *client.Notification, m *NtfyModel) {
			m.ServerURL = types.StringValue(n.Ntfy.ServerURL)
			m.Topic = types.StringValue(n.Ntfy.Topic)
			m.AccessToken = strOrNull(n.Ntfy.AccessToken)
			m.Priority = types.Int64Value(n.Ntfy.Priority)
		},
		Secrets: []secret[NtfyModel]{{
			name:     "access_token",
			plain:    func(m *NtfyModel) *types.String { return &m.AccessToken },
			wo:       func(m *NtfyModel) *types.String { return &m.AccessTokenWo },
			version:  func(m *NtfyModel) *types.Int64 { return &m.AccessTokenWoVersion },
			stored:   func(n *client.Notification) string { return n.Ntfy.AccessToken },
			optional: true,
		}},
	}
}

// ---------------------------------------------------------------- mattermost

type MattermostModel struct {
	Common
	WebhookURL          types.String `tfsdk:"webhook_url"`
	WebhookURLWo        types.String `tfsdk:"webhook_url_wo"`
	WebhookURLWoVersion types.Int64  `tfsdk:"webhook_url_wo_version"`
	Channel             types.String `tfsdk:"channel"`
	Username            types.String `tfsdk:"username"`
}

func MattermostKind() Kind[MattermostModel] {
	return Kind[MattermostModel]{
		Name: "mattermost_notification", Label: "Mattermost", Type: "mattermost",
		Intro: "A Mattermost notification channel (Settings > Notifications). Dokploy posts each message to an incoming webhook.",
		Attributes: map[string]schema.Attribute{
			"webhook_url": secretAttribute("Incoming webhook URL. Set this attribute or `webhook_url_wo`."),
			"channel": schema.StringAttribute{
				Optional:    true,
				Description: "Channel that receives the message. Omit it to use the webhook's default channel. If you remove it from the configuration, the provider clears it.",
			},
			"username": schema.StringAttribute{
				Optional:    true,
				Description: "Display name of the poster. Omit it to use the webhook's default. If you remove it from the configuration, the provider clears it.",
			},
		},
		Common: func(m *MattermostModel) *Common { return &m.Common },
		Create: func(ctx context.Context, c *client.Client, m *MattermostModel) (*client.Notification, error) {
			return c.CreateMattermostNotification(ctx, client.CreateMattermostNotificationRequest{
				NotificationBase: m.base(), WebhookURL: m.WebhookURL.ValueString(),
				Channel: m.Channel.ValueString(), Username: m.Username.ValueString(),
			})
		},
		Update: func(ctx context.Context, c *client.Client, m *MattermostModel, n *client.Notification) error {
			return c.UpdateMattermostNotification(ctx, client.UpdateMattermostNotificationRequest{
				NotificationID: m.ID.ValueString(), MattermostID: n.Mattermost.MattermostID,
				NotificationBase: m.base(), WebhookURL: m.WebhookURL.ValueString(),
				Channel: m.Channel.ValueString(), Username: m.Username.ValueString(),
			})
		},
		Flatten: func(n *client.Notification, m *MattermostModel) {
			m.WebhookURL = types.StringValue(n.Mattermost.WebhookURL)
			m.Channel = strOrNull(n.Mattermost.Channel)
			m.Username = strOrNull(n.Mattermost.Username)
		},
		Secrets: []secret[MattermostModel]{{
			name:    "webhook_url",
			plain:   func(m *MattermostModel) *types.String { return &m.WebhookURL },
			wo:      func(m *MattermostModel) *types.String { return &m.WebhookURLWo },
			version: func(m *MattermostModel) *types.Int64 { return &m.WebhookURLWoVersion },
			stored:  func(n *client.Notification) string { return n.Mattermost.WebhookURL },
		}},
	}
}

// ---------------------------------------------------------------- lark

type LarkModel struct {
	Common
	WebhookURL          types.String `tfsdk:"webhook_url"`
	WebhookURLWo        types.String `tfsdk:"webhook_url_wo"`
	WebhookURLWoVersion types.Int64  `tfsdk:"webhook_url_wo_version"`
}

func LarkKind() Kind[LarkModel] {
	return Kind[LarkModel]{
		Name: "lark_notification", Label: "Lark", Type: "lark",
		Intro: "A Lark (Feishu) notification channel (Settings > Notifications). Dokploy posts each message to a group bot webhook.",
		Attributes: map[string]schema.Attribute{
			"webhook_url": secretAttribute("Webhook URL of the group bot. Set this attribute or `webhook_url_wo`."),
		},
		Common: func(m *LarkModel) *Common { return &m.Common },
		Create: func(ctx context.Context, c *client.Client, m *LarkModel) (*client.Notification, error) {
			return c.CreateLarkNotification(ctx, client.CreateLarkNotificationRequest{
				NotificationBase: m.base(), WebhookURL: m.WebhookURL.ValueString(),
			})
		},
		Update: func(ctx context.Context, c *client.Client, m *LarkModel, n *client.Notification) error {
			return c.UpdateLarkNotification(ctx, client.UpdateLarkNotificationRequest{
				NotificationID: m.ID.ValueString(), LarkID: n.Lark.LarkID,
				NotificationBase: m.base(), WebhookURL: m.WebhookURL.ValueString(),
			})
		},
		Flatten: func(n *client.Notification, m *LarkModel) { m.WebhookURL = types.StringValue(n.Lark.WebhookURL) },
		Secrets: []secret[LarkModel]{{
			name:    "webhook_url",
			plain:   func(m *LarkModel) *types.String { return &m.WebhookURL },
			wo:      func(m *LarkModel) *types.String { return &m.WebhookURLWo },
			version: func(m *LarkModel) *types.Int64 { return &m.WebhookURLWoVersion },
			stored:  func(n *client.Notification) string { return n.Lark.WebhookURL },
		}},
	}
}

// ---------------------------------------------------------------- teams

type TeamsModel struct {
	Common
	WebhookURL          types.String `tfsdk:"webhook_url"`
	WebhookURLWo        types.String `tfsdk:"webhook_url_wo"`
	WebhookURLWoVersion types.Int64  `tfsdk:"webhook_url_wo_version"`
}

func TeamsKind() Kind[TeamsModel] {
	return Kind[TeamsModel]{
		Name: "teams_notification", Label: "Microsoft Teams", Type: "teams",
		Intro: "A Microsoft Teams notification channel (Settings > Notifications). Dokploy posts each message to an incoming webhook.",
		Attributes: map[string]schema.Attribute{
			"webhook_url": secretAttribute("Incoming webhook URL of the Teams channel. Set this attribute or `webhook_url_wo`."),
		},
		Common: func(m *TeamsModel) *Common { return &m.Common },
		Create: func(ctx context.Context, c *client.Client, m *TeamsModel) (*client.Notification, error) {
			return c.CreateTeamsNotification(ctx, client.CreateTeamsNotificationRequest{
				NotificationBase: m.base(), WebhookURL: m.WebhookURL.ValueString(),
			})
		},
		Update: func(ctx context.Context, c *client.Client, m *TeamsModel, n *client.Notification) error {
			return c.UpdateTeamsNotification(ctx, client.UpdateTeamsNotificationRequest{
				NotificationID: m.ID.ValueString(), TeamsID: n.Teams.TeamsID,
				NotificationBase: m.base(), WebhookURL: m.WebhookURL.ValueString(),
			})
		},
		Flatten: func(n *client.Notification, m *TeamsModel) { m.WebhookURL = types.StringValue(n.Teams.WebhookURL) },
		Secrets: []secret[TeamsModel]{{
			name:    "webhook_url",
			plain:   func(m *TeamsModel) *types.String { return &m.WebhookURL },
			wo:      func(m *TeamsModel) *types.String { return &m.WebhookURLWo },
			version: func(m *TeamsModel) *types.Int64 { return &m.WebhookURLWoVersion },
			stored:  func(n *client.Notification) string { return n.Teams.WebhookURL },
		}},
	}
}

// ---------------------------------------------------------------- pushover

type PushoverModel struct {
	Common
	UserKey           types.String `tfsdk:"user_key"`
	UserKeyWo         types.String `tfsdk:"user_key_wo"`
	UserKeyWoVersion  types.Int64  `tfsdk:"user_key_wo_version"`
	APIToken          types.String `tfsdk:"api_token"`
	APITokenWo        types.String `tfsdk:"api_token_wo"`
	APITokenWoVersion types.Int64  `tfsdk:"api_token_wo_version"`
	Priority          types.Int64  `tfsdk:"priority"`
	Retry             types.Int64  `tfsdk:"retry"`
	Expire            types.Int64  `tfsdk:"expire"`
}

func PushoverKind() Kind[PushoverModel] {
	return Kind[PushoverModel]{
		Name: "pushover_notification", Label: "Pushover", Type: "pushover",
		Intro: "A Pushover notification channel (Settings > Notifications). Dokploy sends each message through the Pushover API to a user or a group.",
		Attributes: map[string]schema.Attribute{
			"user_key":  secretAttribute("User or group key from the Pushover dashboard. Set this attribute or `user_key_wo`."),
			"api_token": secretAttribute("API token of the Pushover application. Set this attribute or `api_token_wo`."),
			"priority": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(0),
				Description: "Message priority from `-2` (lowest) to `2` (emergency). Defaults to `0`. Priority `2` needs `retry` and `expire`.",
				Validators:  []validator.Int64{int64validator.Between(-2, 2)},
			},
			"retry": schema.Int64Attribute{
				Optional:    true,
				Description: "Seconds between repeats of an emergency message, 30 or more. Only for priority `2`.",
				Validators:  []validator.Int64{int64validator.AtLeast(30)},
			},
			"expire": schema.Int64Attribute{
				Optional:    true,
				Description: "Seconds after which an emergency message stops repeating, 1 to 10800. Only for priority `2`.",
				Validators:  []validator.Int64{int64validator.Between(1, 10800)},
			},
		},
		Common: func(m *PushoverModel) *Common { return &m.Common },
		Create: func(ctx context.Context, c *client.Client, m *PushoverModel) (*client.Notification, error) {
			return c.CreatePushoverNotification(ctx, client.CreatePushoverNotificationRequest{
				NotificationBase: m.base(), UserKey: m.UserKey.ValueString(), APIToken: m.APIToken.ValueString(),
				Priority: m.Priority.ValueInt64(), Retry: m.Retry.ValueInt64Pointer(), Expire: m.Expire.ValueInt64Pointer(),
			})
		},
		Update: func(ctx context.Context, c *client.Client, m *PushoverModel, n *client.Notification) error {
			return c.UpdatePushoverNotification(ctx, client.UpdatePushoverNotificationRequest{
				NotificationID: m.ID.ValueString(), PushoverID: n.Pushover.PushoverID,
				NotificationBase: m.base(), UserKey: m.UserKey.ValueString(), APIToken: m.APIToken.ValueString(),
				Priority: m.Priority.ValueInt64(), Retry: m.Retry.ValueInt64Pointer(), Expire: m.Expire.ValueInt64Pointer(),
			})
		},
		Flatten: func(n *client.Notification, m *PushoverModel) {
			m.UserKey = types.StringValue(n.Pushover.UserKey)
			m.APIToken = types.StringValue(n.Pushover.APIToken)
			m.Priority = types.Int64Value(n.Pushover.Priority)
			m.Retry = types.Int64PointerValue(n.Pushover.Retry)
			m.Expire = types.Int64PointerValue(n.Pushover.Expire)
		},
		Secrets: []secret[PushoverModel]{
			{
				name:    "user_key",
				plain:   func(m *PushoverModel) *types.String { return &m.UserKey },
				wo:      func(m *PushoverModel) *types.String { return &m.UserKeyWo },
				version: func(m *PushoverModel) *types.Int64 { return &m.UserKeyWoVersion },
				stored:  func(n *client.Notification) string { return n.Pushover.UserKey },
			},
			{
				name:    "api_token",
				plain:   func(m *PushoverModel) *types.String { return &m.APIToken },
				wo:      func(m *PushoverModel) *types.String { return &m.APITokenWo },
				version: func(m *PushoverModel) *types.Int64 { return &m.APITokenWoVersion },
				stored:  func(n *client.Notification) string { return n.Pushover.APIToken },
			},
		},
	}
}

// ---------------------------------------------------------------- custom

type CustomModel struct {
	Common
	Endpoint types.String `tfsdk:"endpoint"`
	Headers  types.Map    `tfsdk:"headers"`
}

// headersOf maps the attribute onto the wire: a null map becomes {}, not
// null, because notification.updateCustom rejects a null record (probed
// live, v0.30.5, 2026-09-05) and stores {} as "no headers".
func headersOf(ctx context.Context, m types.Map) map[string]string {
	out := map[string]string{}
	if m.IsNull() || m.IsUnknown() {
		return out
	}
	_ = m.ElementsAs(ctx, &out, false)
	return out
}

func CustomKind() Kind[CustomModel] {
	return Kind[CustomModel]{
		Name: "custom_notification", Label: "Custom webhook", Type: "custom",
		Intro: "A custom webhook notification channel (Settings > Notifications). Dokploy sends each message as a JSON POST to an endpoint of your own.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{Required: true, Description: "URL that receives the POST."},
			"headers": schema.MapAttribute{
				Optional: true, Sensitive: true, ElementType: types.StringType,
				Description: "HTTP headers that each request carries, for example an authorization header. The map is " +
					"sensitive because it usually holds a credential. If you remove it from the configuration, the provider clears it.",
			},
		},
		Common: func(m *CustomModel) *Common { return &m.Common },
		Create: func(ctx context.Context, c *client.Client, m *CustomModel) (*client.Notification, error) {
			return c.CreateCustomNotification(ctx, client.CreateCustomNotificationRequest{
				NotificationBase: m.base(), Endpoint: m.Endpoint.ValueString(), Headers: headersOf(ctx, m.Headers),
			})
		},
		Update: func(ctx context.Context, c *client.Client, m *CustomModel, n *client.Notification) error {
			return c.UpdateCustomNotification(ctx, client.UpdateCustomNotificationRequest{
				NotificationID: m.ID.ValueString(), CustomID: n.Custom.CustomID,
				NotificationBase: m.base(), Endpoint: m.Endpoint.ValueString(), Headers: headersOf(ctx, m.Headers),
			})
		},
		Flatten: func(n *client.Notification, m *CustomModel) {
			m.Endpoint = types.StringValue(n.Custom.Endpoint)
			if len(n.Custom.Headers) == 0 {
				m.Headers = types.MapNull(types.StringType)
			} else {
				m.Headers, _ = types.MapValueFrom(context.Background(), types.StringType, n.Custom.Headers)
			}
		},
	}
}
