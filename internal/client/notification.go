package client

import (
	"context"
	"net/url"
)

// NotificationEvents are the eight triggers that every channel shares. The
// server stores each as NOT NULL with a false default.
type NotificationEvents struct {
	AppDeploy       bool `json:"appDeploy"`
	AppBuildError   bool `json:"appBuildError"`
	DatabaseBackup  bool `json:"databaseBackup"`
	DockerCleanup   bool `json:"dockerCleanup"`
	DokployRestart  bool `json:"dokployRestart"`
	DokployBackup   bool `json:"dokployBackup"`
	VolumeBackup    bool `json:"volumeBackup"`
	ServerThreshold bool `json:"serverThreshold"`
}

// Notification is one notification channel as notification.one and
// notification.all return it: the shared record plus the channel record
// nested under its type name, all other channel slots null.
//
// Shape captured live (v0.30.5, 2026-09-05). Every channel secret (webhook
// URLs, bot tokens, SMTP passwords, API keys) comes back in CLEARTEXT.
type Notification struct {
	NotificationID string `json:"notificationId"`
	Name           string `json:"name"`
	NotificationEvents
	NotificationType string `json:"notificationType"`
	CreatedAt        string `json:"createdAt"`
	OrganizationID   string `json:"organizationId"`

	Slack      *SlackNotification      `json:"slack"`
	Telegram   *TelegramNotification   `json:"telegram"`
	Discord    *DiscordNotification    `json:"discord"`
	Email      *EmailNotification      `json:"email"`
	Resend     *ResendNotification     `json:"resend"`
	Gotify     *GotifyNotification     `json:"gotify"`
	Ntfy       *NtfyNotification       `json:"ntfy"`
	Mattermost *MattermostNotification `json:"mattermost"`
	Custom     *CustomNotification     `json:"custom"`
	Lark       *LarkNotification       `json:"lark"`
	Pushover   *PushoverNotification   `json:"pushover"`
	Teams      *TeamsNotification      `json:"teams"`
}

type SlackNotification struct {
	SlackID    string `json:"slackId"`
	WebhookURL string `json:"webhookUrl"`
	Channel    string `json:"channel"`
}

type TelegramNotification struct {
	TelegramID      string `json:"telegramId"`
	BotToken        string `json:"botToken"`
	ChatID          string `json:"chatId"`
	MessageThreadID string `json:"messageThreadId"`
}

type DiscordNotification struct {
	DiscordID  string `json:"discordId"`
	WebhookURL string `json:"webhookUrl"`
	Decoration bool   `json:"decoration"`
}

type EmailNotification struct {
	EmailID     string   `json:"emailId"`
	SMTPServer  string   `json:"smtpServer"`
	SMTPPort    int64    `json:"smtpPort"`
	Username    string   `json:"username"`
	Password    string   `json:"password"`
	FromAddress string   `json:"fromAddress"`
	ToAddresses []string `json:"toAddresses"`
}

type ResendNotification struct {
	ResendID    string   `json:"resendId"`
	APIKey      string   `json:"apiKey"`
	FromAddress string   `json:"fromAddress"`
	ToAddresses []string `json:"toAddresses"`
}

type GotifyNotification struct {
	GotifyID   string `json:"gotifyId"`
	ServerURL  string `json:"serverUrl"`
	AppToken   string `json:"appToken"`
	Priority   int64  `json:"priority"`
	Decoration bool   `json:"decoration"`
}

type NtfyNotification struct {
	NtfyID      string `json:"ntfyId"`
	ServerURL   string `json:"serverUrl"`
	Topic       string `json:"topic"`
	AccessToken string `json:"accessToken"`
	Priority    int64  `json:"priority"`
}

type MattermostNotification struct {
	MattermostID string `json:"mattermostId"`
	WebhookURL   string `json:"webhookUrl"`
	Channel      string `json:"channel"`
	Username     string `json:"username"`
}

type CustomNotification struct {
	CustomID string            `json:"customId"`
	Endpoint string            `json:"endpoint"`
	Headers  map[string]string `json:"headers"`
}

type LarkNotification struct {
	LarkID     string `json:"larkId"`
	WebhookURL string `json:"webhookUrl"`
}

// PushoverNotification. retry and expire are nullable on the server and
// only required for the emergency priority (2).
type PushoverNotification struct {
	PushoverID string `json:"pushoverId"`
	UserKey    string `json:"userKey"`
	APIToken   string `json:"apiToken"`
	Priority   int64  `json:"priority"`
	Retry      *int64 `json:"retry"`
	Expire     *int64 `json:"expire"`
}

type TeamsNotification struct {
	TeamsID    string `json:"teamsId"`
	WebhookURL string `json:"webhookUrl"`
}

// NotificationBase is the shared part of every create and update request.
// The resource sends every field on every call: a create<Type> requires the
// full set, an update<Type> keeps an absent key, rejects a null string, and
// answers 500 "No values to set" to a body that carries only name (probed
// live, v0.30.5, 2026-09-05).
type NotificationBase struct {
	Name string `json:"name"`
	NotificationEvents
}

type CreateSlackNotificationRequest struct {
	NotificationBase
	WebhookURL string `json:"webhookUrl"`
	Channel    string `json:"channel"`
}

type UpdateSlackNotificationRequest struct {
	NotificationID string `json:"notificationId"`
	SlackID        string `json:"slackId"`
	NotificationBase
	WebhookURL string `json:"webhookUrl"`
	Channel    string `json:"channel"`
}

type CreateTelegramNotificationRequest struct {
	NotificationBase
	BotToken        string `json:"botToken"`
	ChatID          string `json:"chatId"`
	MessageThreadID string `json:"messageThreadId"`
}

type UpdateTelegramNotificationRequest struct {
	NotificationID string `json:"notificationId"`
	TelegramID     string `json:"telegramId"`
	NotificationBase
	BotToken        string `json:"botToken"`
	ChatID          string `json:"chatId"`
	MessageThreadID string `json:"messageThreadId"`
}

type CreateDiscordNotificationRequest struct {
	NotificationBase
	WebhookURL string `json:"webhookUrl"`
	Decoration bool   `json:"decoration"`
}

type UpdateDiscordNotificationRequest struct {
	NotificationID string `json:"notificationId"`
	DiscordID      string `json:"discordId"`
	NotificationBase
	WebhookURL string `json:"webhookUrl"`
	Decoration bool   `json:"decoration"`
}

type CreateEmailNotificationRequest struct {
	NotificationBase
	SMTPServer  string   `json:"smtpServer"`
	SMTPPort    int64    `json:"smtpPort"`
	Username    string   `json:"username"`
	Password    string   `json:"password"`
	FromAddress string   `json:"fromAddress"`
	ToAddresses []string `json:"toAddresses"`
}

type UpdateEmailNotificationRequest struct {
	NotificationID string `json:"notificationId"`
	EmailID        string `json:"emailId"`
	NotificationBase
	SMTPServer  string   `json:"smtpServer"`
	SMTPPort    int64    `json:"smtpPort"`
	Username    string   `json:"username"`
	Password    string   `json:"password"`
	FromAddress string   `json:"fromAddress"`
	ToAddresses []string `json:"toAddresses"`
}

type CreateResendNotificationRequest struct {
	NotificationBase
	APIKey      string   `json:"apiKey"`
	FromAddress string   `json:"fromAddress"`
	ToAddresses []string `json:"toAddresses"`
}

type UpdateResendNotificationRequest struct {
	NotificationID string `json:"notificationId"`
	ResendID       string `json:"resendId"`
	NotificationBase
	APIKey      string   `json:"apiKey"`
	FromAddress string   `json:"fromAddress"`
	ToAddresses []string `json:"toAddresses"`
}

type CreateGotifyNotificationRequest struct {
	NotificationBase
	ServerURL  string `json:"serverUrl"`
	AppToken   string `json:"appToken"`
	Priority   int64  `json:"priority"`
	Decoration bool   `json:"decoration"`
}

type UpdateGotifyNotificationRequest struct {
	NotificationID string `json:"notificationId"`
	GotifyID       string `json:"gotifyId"`
	NotificationBase
	ServerURL  string `json:"serverUrl"`
	AppToken   string `json:"appToken"`
	Priority   int64  `json:"priority"`
	Decoration bool   `json:"decoration"`
}

type CreateNtfyNotificationRequest struct {
	NotificationBase
	ServerURL   string `json:"serverUrl"`
	Topic       string `json:"topic"`
	AccessToken string `json:"accessToken"`
	Priority    int64  `json:"priority"`
}

type UpdateNtfyNotificationRequest struct {
	NotificationID string `json:"notificationId"`
	NtfyID         string `json:"ntfyId"`
	NotificationBase
	ServerURL   string `json:"serverUrl"`
	Topic       string `json:"topic"`
	AccessToken string `json:"accessToken"`
	Priority    int64  `json:"priority"`
}

type CreateMattermostNotificationRequest struct {
	NotificationBase
	WebhookURL string `json:"webhookUrl"`
	Channel    string `json:"channel"`
	Username   string `json:"username"`
}

type UpdateMattermostNotificationRequest struct {
	NotificationID string `json:"notificationId"`
	MattermostID   string `json:"mattermostId"`
	NotificationBase
	WebhookURL string `json:"webhookUrl"`
	Channel    string `json:"channel"`
	Username   string `json:"username"`
}

type CreateCustomNotificationRequest struct {
	NotificationBase
	Endpoint string            `json:"endpoint"`
	Headers  map[string]string `json:"headers"`
}

type UpdateCustomNotificationRequest struct {
	NotificationID string `json:"notificationId"`
	CustomID       string `json:"customId"`
	NotificationBase
	Endpoint string            `json:"endpoint"`
	Headers  map[string]string `json:"headers"`
}

type CreateLarkNotificationRequest struct {
	NotificationBase
	WebhookURL string `json:"webhookUrl"`
}

type UpdateLarkNotificationRequest struct {
	NotificationID string `json:"notificationId"`
	LarkID         string `json:"larkId"`
	NotificationBase
	WebhookURL string `json:"webhookUrl"`
}

// CreatePushoverNotificationRequest. Retry and Expire are pointers: the
// schema takes null for them, and the emergency priority (2) requires both.
type CreatePushoverNotificationRequest struct {
	NotificationBase
	UserKey  string `json:"userKey"`
	APIToken string `json:"apiToken"`
	Priority int64  `json:"priority"`
	Retry    *int64 `json:"retry"`
	Expire   *int64 `json:"expire"`
}

type UpdatePushoverNotificationRequest struct {
	NotificationID string `json:"notificationId"`
	PushoverID     string `json:"pushoverId"`
	NotificationBase
	UserKey  string `json:"userKey"`
	APIToken string `json:"apiToken"`
	Priority int64  `json:"priority"`
	Retry    *int64 `json:"retry"`
	Expire   *int64 `json:"expire"`
}

type CreateTeamsNotificationRequest struct {
	NotificationBase
	WebhookURL string `json:"webhookUrl"`
}

type UpdateTeamsNotificationRequest struct {
	NotificationID string `json:"notificationId"`
	TeamsID        string `json:"teamsId"`
	NotificationBase
	WebhookURL string `json:"webhookUrl"`
}

func (c *Client) GetNotification(ctx context.Context, id string) (*Notification, error) {
	var n Notification
	if err := c.Get(ctx, "/notification.one", url.Values{"notificationId": {id}}, &n); err != nil {
		return nil, err
	}
	return &n, nil
}

func (c *Client) ListNotifications(ctx context.Context) ([]Notification, error) {
	var ns []Notification
	if err := c.Get(ctx, "/notification.all", nil, &ns); err != nil {
		return nil, err
	}
	return ns, nil
}

// DeleteNotification. Note the verb: notification uses .remove, for every
// channel type.
func (c *Client) DeleteNotification(ctx context.Context, id string) error {
	return c.Post(ctx, "/notification.remove", map[string]string{"notificationId": id}, nil)
}

// createNotification posts one create<Type> body and returns the record.
// Every create<Type> answers HTTP 200 with an EMPTY body and names are not
// unique, so the id comes from the notification.all diff, filtered to the
// channel type and told apart from a sibling by match (locateCreated).
func (c *Client) createNotification(ctx context.Context, kind, path string, req any, match func(Notification) bool) (*Notification, error) {
	id, err := locateCreated(ctx, "notification", kind+" notification",
		func(ctx context.Context) ([]Notification, error) {
			all, err := c.ListNotifications(ctx)
			if err != nil {
				return nil, err
			}
			var out []Notification
			for _, n := range all {
				if n.NotificationType == kind {
					out = append(out, n)
				}
			}
			return out, nil
		},
		func(ctx context.Context) error { return c.Post(ctx, path, req, nil) },
		func(n Notification) string { return n.NotificationID },
		match,
	)
	if err != nil {
		return nil, err
	}
	return c.GetNotification(ctx, id)
}

func (c *Client) CreateSlackNotification(ctx context.Context, req CreateSlackNotificationRequest) (*Notification, error) {
	return c.createNotification(ctx, "slack", "/notification.createSlack", req, func(n Notification) bool {
		return n.Name == req.Name && n.Slack != nil && n.Slack.WebhookURL == req.WebhookURL
	})
}

func (c *Client) UpdateSlackNotification(ctx context.Context, req UpdateSlackNotificationRequest) error {
	return c.Post(ctx, "/notification.updateSlack", req, nil)
}

func (c *Client) CreateTelegramNotification(ctx context.Context, req CreateTelegramNotificationRequest) (*Notification, error) {
	return c.createNotification(ctx, "telegram", "/notification.createTelegram", req, func(n Notification) bool {
		return n.Name == req.Name && n.Telegram != nil && n.Telegram.ChatID == req.ChatID
	})
}

func (c *Client) UpdateTelegramNotification(ctx context.Context, req UpdateTelegramNotificationRequest) error {
	return c.Post(ctx, "/notification.updateTelegram", req, nil)
}

func (c *Client) CreateDiscordNotification(ctx context.Context, req CreateDiscordNotificationRequest) (*Notification, error) {
	return c.createNotification(ctx, "discord", "/notification.createDiscord", req, func(n Notification) bool {
		return n.Name == req.Name && n.Discord != nil && n.Discord.WebhookURL == req.WebhookURL
	})
}

func (c *Client) UpdateDiscordNotification(ctx context.Context, req UpdateDiscordNotificationRequest) error {
	return c.Post(ctx, "/notification.updateDiscord", req, nil)
}

func (c *Client) CreateEmailNotification(ctx context.Context, req CreateEmailNotificationRequest) (*Notification, error) {
	return c.createNotification(ctx, "email", "/notification.createEmail", req, func(n Notification) bool {
		return n.Name == req.Name && n.Email != nil && n.Email.SMTPServer == req.SMTPServer && n.Email.FromAddress == req.FromAddress
	})
}

func (c *Client) UpdateEmailNotification(ctx context.Context, req UpdateEmailNotificationRequest) error {
	return c.Post(ctx, "/notification.updateEmail", req, nil)
}

func (c *Client) CreateResendNotification(ctx context.Context, req CreateResendNotificationRequest) (*Notification, error) {
	return c.createNotification(ctx, "resend", "/notification.createResend", req, func(n Notification) bool {
		return n.Name == req.Name && n.Resend != nil && n.Resend.FromAddress == req.FromAddress
	})
}

func (c *Client) UpdateResendNotification(ctx context.Context, req UpdateResendNotificationRequest) error {
	return c.Post(ctx, "/notification.updateResend", req, nil)
}

func (c *Client) CreateGotifyNotification(ctx context.Context, req CreateGotifyNotificationRequest) (*Notification, error) {
	return c.createNotification(ctx, "gotify", "/notification.createGotify", req, func(n Notification) bool {
		return n.Name == req.Name && n.Gotify != nil && n.Gotify.ServerURL == req.ServerURL
	})
}

func (c *Client) UpdateGotifyNotification(ctx context.Context, req UpdateGotifyNotificationRequest) error {
	return c.Post(ctx, "/notification.updateGotify", req, nil)
}

func (c *Client) CreateNtfyNotification(ctx context.Context, req CreateNtfyNotificationRequest) (*Notification, error) {
	return c.createNotification(ctx, "ntfy", "/notification.createNtfy", req, func(n Notification) bool {
		return n.Name == req.Name && n.Ntfy != nil && n.Ntfy.ServerURL == req.ServerURL && n.Ntfy.Topic == req.Topic
	})
}

func (c *Client) UpdateNtfyNotification(ctx context.Context, req UpdateNtfyNotificationRequest) error {
	return c.Post(ctx, "/notification.updateNtfy", req, nil)
}

func (c *Client) CreateMattermostNotification(ctx context.Context, req CreateMattermostNotificationRequest) (*Notification, error) {
	return c.createNotification(ctx, "mattermost", "/notification.createMattermost", req, func(n Notification) bool {
		return n.Name == req.Name && n.Mattermost != nil && n.Mattermost.WebhookURL == req.WebhookURL
	})
}

func (c *Client) UpdateMattermostNotification(ctx context.Context, req UpdateMattermostNotificationRequest) error {
	return c.Post(ctx, "/notification.updateMattermost", req, nil)
}

func (c *Client) CreateCustomNotification(ctx context.Context, req CreateCustomNotificationRequest) (*Notification, error) {
	return c.createNotification(ctx, "custom", "/notification.createCustom", req, func(n Notification) bool {
		return n.Name == req.Name && n.Custom != nil && n.Custom.Endpoint == req.Endpoint
	})
}

func (c *Client) UpdateCustomNotification(ctx context.Context, req UpdateCustomNotificationRequest) error {
	return c.Post(ctx, "/notification.updateCustom", req, nil)
}

func (c *Client) CreateLarkNotification(ctx context.Context, req CreateLarkNotificationRequest) (*Notification, error) {
	return c.createNotification(ctx, "lark", "/notification.createLark", req, func(n Notification) bool {
		return n.Name == req.Name && n.Lark != nil && n.Lark.WebhookURL == req.WebhookURL
	})
}

func (c *Client) UpdateLarkNotification(ctx context.Context, req UpdateLarkNotificationRequest) error {
	return c.Post(ctx, "/notification.updateLark", req, nil)
}

func (c *Client) CreatePushoverNotification(ctx context.Context, req CreatePushoverNotificationRequest) (*Notification, error) {
	return c.createNotification(ctx, "pushover", "/notification.createPushover", req, func(n Notification) bool {
		return n.Name == req.Name && n.Pushover != nil && n.Pushover.UserKey == req.UserKey
	})
}

func (c *Client) UpdatePushoverNotification(ctx context.Context, req UpdatePushoverNotificationRequest) error {
	return c.Post(ctx, "/notification.updatePushover", req, nil)
}

func (c *Client) CreateTeamsNotification(ctx context.Context, req CreateTeamsNotificationRequest) (*Notification, error) {
	return c.createNotification(ctx, "teams", "/notification.createTeams", req, func(n Notification) bool {
		return n.Name == req.Name && n.Teams != nil && n.Teams.WebhookURL == req.WebhookURL
	})
}

func (c *Client) UpdateTeamsNotification(ctx context.Context, req UpdateTeamsNotificationRequest) error {
	return c.Post(ctx, "/notification.updateTeams", req, nil)
}
