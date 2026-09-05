package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

// notificationJSON is the exact shape notification.one returns for a Slack
// channel, captured live (v0.30.5, 2026-09-05); the other channel slots are
// null.
const notificationJSON = `{
	"notificationId": "n1",
	"name": "ops",
	"appDeploy": true,
	"appBuildError": false,
	"databaseBackup": true,
	"volumeBackup": false,
	"dokployRestart": false,
	"dokployBackup": true,
	"dockerCleanup": false,
	"serverThreshold": true,
	"notificationType": "slack",
	"createdAt": "2026-09-05T15:49:09.836Z",
	"slackId": "s1",
	"telegramId": null,
	"organizationId": "org1",
	"slack": {"slackId": "s1", "webhookUrl": "https://hooks.slack.com/services/x", "channel": "#ops"},
	"telegram": null,
	"discord": null,
	"email": null,
	"resend": null,
	"gotify": null,
	"ntfy": null,
	"mattermost": null,
	"custom": null,
	"lark": null,
	"pushover": null,
	"teams": null
}`

func TestCreateSlackNotificationLocatesTheRecord(t *testing.T) {
	srv := locateServer(t, "/api/notification.all", "/api/notification.createSlack", "/api/notification.one", notificationJSON, "")
	defer srv.Close()
	c := testClient(t, srv)

	n, err := c.CreateSlackNotification(context.Background(), CreateSlackNotificationRequest{
		NotificationBase: NotificationBase{Name: "ops"},
		WebhookURL:       "https://hooks.slack.com/services/x",
	})
	if err != nil {
		t.Fatalf("CreateSlackNotification: %v", err)
	}
	if n.NotificationID != "n1" || n.Name != "ops" || !n.AppDeploy || n.AppBuildError || !n.DatabaseBackup || n.VolumeBackup ||
		n.DokployRestart || !n.DokployBackup || n.DockerCleanup || !n.ServerThreshold || n.NotificationType != "slack" ||
		n.CreatedAt != "2026-09-05T15:49:09.836Z" || n.OrganizationID != "org1" {
		t.Errorf("notification = %+v", n)
	}
	if n.Slack == nil || n.Slack.SlackID != "s1" || n.Slack.WebhookURL != "https://hooks.slack.com/services/x" || n.Slack.Channel != "#ops" {
		t.Errorf("slack = %+v", n.Slack)
	}
	if n.Telegram != nil || n.Email != nil || n.Pushover != nil {
		t.Errorf("other channels must decode to nil, got %+v", n)
	}
}

// TestNotificationChannelsDecodeEveryField pins one fixture per channel
// record, so a typo'd tag on any of the twelve cannot stay green.
func TestNotificationChannelsDecodeEveryField(t *testing.T) {
	raw := `{"notificationId":"n","name":"n","notificationType":"x",
	"telegram":{"telegramId":"t1","botToken":"bt","chatId":"c1","messageThreadId":"7"},
	"discord":{"discordId":"d1","webhookUrl":"https://d","decoration":true},
	"email":{"emailId":"e1","smtpServer":"smtp","smtpPort":587,"username":"u","password":"p","fromAddress":"a@x","toAddresses":["b@x","c@x"]},
	"resend":{"resendId":"r1","apiKey":"re","fromAddress":"a@x","toAddresses":["b@x"]},
	"gotify":{"gotifyId":"g1","serverUrl":"https://g","appToken":"tok","priority":5,"decoration":false},
	"ntfy":{"ntfyId":"nt1","serverUrl":"https://n","topic":"t","accessToken":"at","priority":3},
	"mattermost":{"mattermostId":"m1","webhookUrl":"https://m","channel":"town","username":"bot"},
	"custom":{"customId":"cu1","endpoint":"https://c","headers":{"X-A":"b"}},
	"lark":{"larkId":"l1","webhookUrl":"https://l"},
	"pushover":{"pushoverId":"p1","userKey":"uk","apiToken":"at","priority":2,"retry":30,"expire":3600},
	"teams":{"teamsId":"tm1","webhookUrl":"https://t"}}`
	var n Notification
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		t.Fatal(err)
	}
	if n.Telegram.TelegramID != "t1" || n.Telegram.BotToken != "bt" || n.Telegram.ChatID != "c1" || n.Telegram.MessageThreadID != "7" {
		t.Errorf("telegram = %+v", n.Telegram)
	}
	if n.Discord.DiscordID != "d1" || n.Discord.WebhookURL != "https://d" || !n.Discord.Decoration {
		t.Errorf("discord = %+v", n.Discord)
	}
	if n.Email.EmailID != "e1" || n.Email.SMTPServer != "smtp" || n.Email.SMTPPort != 587 || n.Email.Username != "u" ||
		n.Email.Password != "p" || n.Email.FromAddress != "a@x" || len(n.Email.ToAddresses) != 2 || n.Email.ToAddresses[1] != "c@x" {
		t.Errorf("email = %+v", n.Email)
	}
	if n.Resend.ResendID != "r1" || n.Resend.APIKey != "re" || n.Resend.FromAddress != "a@x" || len(n.Resend.ToAddresses) != 1 {
		t.Errorf("resend = %+v", n.Resend)
	}
	if n.Gotify.GotifyID != "g1" || n.Gotify.ServerURL != "https://g" || n.Gotify.AppToken != "tok" || n.Gotify.Priority != 5 || n.Gotify.Decoration {
		t.Errorf("gotify = %+v", n.Gotify)
	}
	if n.Ntfy.NtfyID != "nt1" || n.Ntfy.ServerURL != "https://n" || n.Ntfy.Topic != "t" || n.Ntfy.AccessToken != "at" || n.Ntfy.Priority != 3 {
		t.Errorf("ntfy = %+v", n.Ntfy)
	}
	if n.Mattermost.MattermostID != "m1" || n.Mattermost.WebhookURL != "https://m" || n.Mattermost.Channel != "town" || n.Mattermost.Username != "bot" {
		t.Errorf("mattermost = %+v", n.Mattermost)
	}
	if n.Custom.CustomID != "cu1" || n.Custom.Endpoint != "https://c" || n.Custom.Headers["X-A"] != "b" {
		t.Errorf("custom = %+v", n.Custom)
	}
	if n.Lark.LarkID != "l1" || n.Lark.WebhookURL != "https://l" {
		t.Errorf("lark = %+v", n.Lark)
	}
	if n.Pushover.PushoverID != "p1" || n.Pushover.UserKey != "uk" || n.Pushover.APIToken != "at" || n.Pushover.Priority != 2 ||
		n.Pushover.Retry == nil || *n.Pushover.Retry != 30 || n.Pushover.Expire == nil || *n.Pushover.Expire != 3600 {
		t.Errorf("pushover = %+v", n.Pushover)
	}
	if n.Teams.TeamsID != "tm1" || n.Teams.WebhookURL != "https://t" {
		t.Errorf("teams = %+v", n.Teams)
	}
}

// TestNotificationRequestsFlattenTheBase pins the embedded-struct encoding:
// the shared fields must land at the top level of the JSON body, where the
// zod schemas expect them.
func TestNotificationRequestsFlattenTheBase(t *testing.T) {
	raw, err := json.Marshal(UpdateSlackNotificationRequest{
		NotificationID:   "n1",
		SlackID:          "s1",
		NotificationBase: NotificationBase{Name: "ops", NotificationEvents: NotificationEvents{AppDeploy: true}},
		WebhookURL:       "https://x",
	})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"notificationId", "slackId", "name", "appDeploy", "serverThreshold", "webhookUrl", "channel"} {
		if _, ok := m[k]; !ok {
			t.Errorf("update body lacks %q: %s", k, raw)
		}
	}
	if _, ok := m["NotificationBase"]; ok {
		t.Errorf("update body nests the base instead of flattening it: %s", raw)
	}
}

func TestGetListDeleteNotification(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodGet, Path: "/api/notification.one", Status: 200, Body: notificationJSON},
		route{Method: http.MethodGet, Path: "/api/notification.all", Status: 200, Body: "[" + notificationJSON + "]"},
		route{Method: http.MethodPost, Path: "/api/notification.remove", Status: 200, Body: notificationJSON},
		route{Method: http.MethodPost, Path: "/api/notification.updateSlack", Status: 200, Body: ""},
	)
	defer srv.Close()
	c := testClient(t, srv)
	ctx := context.Background()
	if got, err := c.GetNotification(ctx, "n1"); err != nil || got.NotificationID != "n1" {
		t.Errorf("GetNotification = %+v, %v", got, err)
	}
	if list, err := c.ListNotifications(ctx); err != nil || len(list) != 1 {
		t.Errorf("ListNotifications = %+v, %v", list, err)
	}
	if err := c.UpdateSlackNotification(ctx, UpdateSlackNotificationRequest{NotificationID: "n1"}); err != nil {
		t.Errorf("UpdateSlackNotification: %v", err)
	}
	if err := c.DeleteNotification(ctx, "n1"); err != nil {
		t.Errorf("DeleteNotification: %v", err)
	}
}

func TestGetNotificationNotFound(t *testing.T) {
	srv := testRoutes(t,
		route{Method: http.MethodGet, Path: "/api/notification.one", Status: 404, Body: `{"message":"Notification not found","code":"NOT_FOUND"}`},
	)
	defer srv.Close()
	c := testClient(t, srv)
	if _, err := c.GetNotification(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetNotification(unknown) = %v, want ErrNotFound", err)
	}
}
