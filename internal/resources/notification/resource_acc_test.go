// Package notification_test holds the acceptance tests (external package;
// acctest imports provider, which imports notification).
package notification_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func checkDestroy(s *terraform.State) error {
	c, err := acctest.ClientFromEnv()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if _, err := c.GetNotification(context.Background(), rs.Primary.ID); !errors.Is(err, client.ErrNotFound) {
			return fmt.Errorf("%s %s still exists (err = %v)", rs.Type, rs.Primary.ID, err)
		}
	}
	return nil
}

// checkServer reads the record back through the API: the channel secrets
// and the event flags must hold on the server, not only in the state.
func checkServer(addr string, assert func(*client.Notification) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[addr]
		if !ok {
			return fmt.Errorf("%s not found in state", addr)
		}
		c, err := acctest.ClientFromEnv()
		if err != nil {
			return err
		}
		n, err := c.GetNotification(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		return assert(n)
	}
}

// channelCase is one channel: a full configuration with every attribute
// and two events on, and a reduced one that drops the optional attributes
// and the events. Dokploy never contacts the channel on create or update,
// so placeholder credentials are fine.
type channelCase struct {
	kind         string
	full         string
	reduced      string
	checkFull    func(*client.Notification) error
	checkReduced func(*client.Notification) error
}

func str(s string) *string { return &s }

var cases = []channelCase{
	{
		kind: "slack",
		full: `  webhook_url = "https://hooks.slack.com/services/T/B/x"
  channel     = "#deploys"`,
		reduced: `  webhook_url = "https://hooks.slack.com/services/T/B/y"`,
		checkFull: func(n *client.Notification) error {
			if n.Slack == nil || n.Slack.WebhookURL != "https://hooks.slack.com/services/T/B/x" || n.Slack.Channel != "#deploys" {
				return fmt.Errorf("slack = %+v", n.Slack)
			}
			return nil
		},
		checkReduced: func(n *client.Notification) error {
			if n.Slack.WebhookURL != "https://hooks.slack.com/services/T/B/y" || n.Slack.Channel != "" {
				return fmt.Errorf("slack = %+v, want the channel cleared", n.Slack)
			}
			return nil
		},
	},
	{
		kind: "discord",
		full: `  webhook_url = "https://discord.com/api/webhooks/1/x"
  decoration  = false`,
		reduced: `  webhook_url = "https://discord.com/api/webhooks/1/x"`,
		checkFull: func(n *client.Notification) error {
			if n.Discord == nil || n.Discord.Decoration {
				return fmt.Errorf("discord = %+v, want decoration false", n.Discord)
			}
			return nil
		},
		checkReduced: func(n *client.Notification) error {
			if !n.Discord.Decoration {
				return fmt.Errorf("discord = %+v, want the decoration default (true)", n.Discord)
			}
			return nil
		},
	},
	{
		kind: "telegram",
		full: `  bot_token         = "123:abc"
  chat_id           = "-100123"
  message_thread_id = "7"`,
		reduced: `  bot_token = "123:abc"
  chat_id   = "-100123"`,
		checkFull: func(n *client.Notification) error {
			if n.Telegram == nil || n.Telegram.BotToken != "123:abc" || n.Telegram.ChatID != "-100123" || n.Telegram.MessageThreadID != "7" {
				return fmt.Errorf("telegram = %+v", n.Telegram)
			}
			return nil
		},
		checkReduced: func(n *client.Notification) error {
			if n.Telegram.MessageThreadID != "" {
				return fmt.Errorf("telegram = %+v, want the thread id cleared", n.Telegram)
			}
			return nil
		},
	},
	{
		kind: "email",
		full: `  smtp_server  = "smtp.example.com"
  smtp_port    = 587
  username     = "mailer"
  password     = "acceptance-only"
  from_address = "dokploy@example.com"
  to_addresses = ["ops@example.com", "dev@example.com"]`,
		reduced: `  smtp_server  = "smtp.example.com"
  smtp_port    = 465
  username     = "mailer"
  password     = "acceptance-only"
  from_address = "dokploy@example.com"
  to_addresses = ["ops@example.com"]`,
		checkFull: func(n *client.Notification) error {
			if n.Email == nil || n.Email.SMTPPort != 587 || n.Email.Password != "acceptance-only" || len(n.Email.ToAddresses) != 2 {
				return fmt.Errorf("email = %+v", n.Email)
			}
			return nil
		},
		checkReduced: func(n *client.Notification) error {
			if n.Email.SMTPPort != 465 || len(n.Email.ToAddresses) != 1 {
				return fmt.Errorf("email = %+v", n.Email)
			}
			return nil
		},
	},
	{
		kind: "resend",
		full: `  api_key      = "re_acceptance"
  from_address = "dokploy@example.com"
  to_addresses = ["ops@example.com"]`,
		reduced: `  api_key      = "re_acceptance_2"
  from_address = "dokploy@example.com"
  to_addresses = ["ops@example.com"]`,
		checkFull: func(n *client.Notification) error {
			if n.Resend == nil || n.Resend.APIKey != "re_acceptance" {
				return fmt.Errorf("resend = %+v", n.Resend)
			}
			return nil
		},
		checkReduced: func(n *client.Notification) error {
			if n.Resend.APIKey != "re_acceptance_2" {
				return fmt.Errorf("resend = %+v", n.Resend)
			}
			return nil
		},
	},
	{
		kind: "gotify",
		full: `  server_url = "https://gotify.example.com"
  app_token  = "tok"
  priority   = 8
  decoration = false`,
		reduced: `  server_url = "https://gotify.example.com"
  app_token  = "tok"`,
		checkFull: func(n *client.Notification) error {
			if n.Gotify == nil || n.Gotify.Priority != 8 || n.Gotify.Decoration {
				return fmt.Errorf("gotify = %+v", n.Gotify)
			}
			return nil
		},
		checkReduced: func(n *client.Notification) error {
			if n.Gotify.Priority != 5 || !n.Gotify.Decoration {
				return fmt.Errorf("gotify = %+v, want the defaults (5, true)", n.Gotify)
			}
			return nil
		},
	},
	{
		kind: "ntfy",
		full: `  server_url   = "https://ntfy.sh"
  topic        = "dokploy-acc"
  access_token = "tk_acceptance"
  priority     = 4`,
		reduced: `  server_url = "https://ntfy.sh"
  topic      = "dokploy-acc"`,
		checkFull: func(n *client.Notification) error {
			if n.Ntfy == nil || n.Ntfy.AccessToken != "tk_acceptance" || n.Ntfy.Priority != 4 {
				return fmt.Errorf("ntfy = %+v", n.Ntfy)
			}
			return nil
		},
		checkReduced: func(n *client.Notification) error {
			if n.Ntfy.AccessToken != "" || n.Ntfy.Priority != 3 {
				return fmt.Errorf("ntfy = %+v, want the token cleared and the default priority", n.Ntfy)
			}
			return nil
		},
	},
	{
		kind: "mattermost",
		full: `  webhook_url = "https://mm.example.com/hooks/x"
  channel     = "town-square"
  username    = "dokploy"`,
		reduced: `  webhook_url = "https://mm.example.com/hooks/x"`,
		checkFull: func(n *client.Notification) error {
			if n.Mattermost == nil || n.Mattermost.Channel != "town-square" || n.Mattermost.Username != "dokploy" {
				return fmt.Errorf("mattermost = %+v", n.Mattermost)
			}
			return nil
		},
		checkReduced: func(n *client.Notification) error {
			if n.Mattermost.Channel != "" || n.Mattermost.Username != "" {
				return fmt.Errorf("mattermost = %+v, want channel and username cleared", n.Mattermost)
			}
			return nil
		},
	},
	{
		kind:    "lark",
		full:    `  webhook_url = "https://open.larksuite.com/open-apis/bot/v2/hook/x"`,
		reduced: `  webhook_url = "https://open.larksuite.com/open-apis/bot/v2/hook/y"`,
		checkFull: func(n *client.Notification) error {
			if n.Lark == nil || n.Lark.WebhookURL != "https://open.larksuite.com/open-apis/bot/v2/hook/x" {
				return fmt.Errorf("lark = %+v", n.Lark)
			}
			return nil
		},
		checkReduced: func(n *client.Notification) error {
			if n.Lark.WebhookURL != "https://open.larksuite.com/open-apis/bot/v2/hook/y" {
				return fmt.Errorf("lark = %+v", n.Lark)
			}
			return nil
		},
	},
	{
		kind:    "teams",
		full:    `  webhook_url = "https://example.webhook.office.com/webhookb2/x"`,
		reduced: `  webhook_url = "https://example.webhook.office.com/webhookb2/y"`,
		checkFull: func(n *client.Notification) error {
			if n.Teams == nil || n.Teams.WebhookURL != "https://example.webhook.office.com/webhookb2/x" {
				return fmt.Errorf("teams = %+v", n.Teams)
			}
			return nil
		},
		checkReduced: func(n *client.Notification) error {
			if n.Teams.WebhookURL != "https://example.webhook.office.com/webhookb2/y" {
				return fmt.Errorf("teams = %+v", n.Teams)
			}
			return nil
		},
	},
	{
		kind: "pushover",
		full: `  user_key  = "uk-acceptance"
  api_token = "at-acceptance"
  priority  = 2
  retry     = 60
  expire    = 3600`,
		reduced: `  user_key  = "uk-acceptance"
  api_token = "at-acceptance"`,
		checkFull: func(n *client.Notification) error {
			if n.Pushover == nil || n.Pushover.Priority != 2 || n.Pushover.Retry == nil || *n.Pushover.Retry != 60 || n.Pushover.Expire == nil || *n.Pushover.Expire != 3600 {
				return fmt.Errorf("pushover = %+v", n.Pushover)
			}
			return nil
		},
		checkReduced: func(n *client.Notification) error {
			if n.Pushover.Priority != 0 || n.Pushover.Retry != nil || n.Pushover.Expire != nil {
				return fmt.Errorf("pushover = %+v, want priority 0 and retry/expire cleared", n.Pushover)
			}
			return nil
		},
	},
	{
		kind: "custom",
		full: `  endpoint = "https://hooks.example.com/dokploy"
  headers = {
    "Authorization" = "Bearer acceptance"
  }`,
		reduced: `  endpoint = "https://hooks.example.com/dokploy"`,
		checkFull: func(n *client.Notification) error {
			if n.Custom == nil || n.Custom.Headers["Authorization"] != "Bearer acceptance" {
				return fmt.Errorf("custom = %+v", n.Custom)
			}
			return nil
		},
		checkReduced: func(n *client.Notification) error {
			if len(n.Custom.Headers) != 0 {
				return fmt.Errorf("custom = %+v, want the headers cleared", n.Custom)
			}
			return nil
		},
	},
}

func channelConfig(kind, name, body, events string) string {
	return fmt.Sprintf(`
resource "dokploy_%s_notification" "test" {
  name = %q
%s
%s
}
`, kind, name, body, events)
}

// TestAccNotification_channels runs the lifecycle of every channel: the
// full shape with two events on, the reduced shape with the events off
// (optional attributes clear on the server, defaults come back), and an
// import that verifies the whole state.
func TestAccNotification_channels(t *testing.T) {
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			addr := "dokploy_" + tc.kind + "_notification.test"
			name := acctest.RandomName("notif-" + tc.kind)
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { acctest.PreCheck(t) },
				ProtoV6ProviderFactories: acctest.ProviderFactories(),
				CheckDestroy:             checkDestroy,
				Steps: []resource.TestStep{
					{
						Config: channelConfig(tc.kind, name, tc.full, "  app_deploy      = true\n  database_backup = true"),
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(addr, "name", name),
							resource.TestCheckResourceAttr(addr, "app_deploy", "true"),
							resource.TestCheckResourceAttr(addr, "app_build_error", "false"),
							resource.TestCheckResourceAttrSet(addr, "created_at"),
							checkServer(addr, func(n *client.Notification) error {
								if n.NotificationType != tc.kind || !n.AppDeploy || !n.DatabaseBackup || n.AppBuildError {
									return fmt.Errorf("record = type %q events %+v", n.NotificationType, n.NotificationEvents)
								}
								return tc.checkFull(n)
							}),
						),
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
						},
					},
					{
						Config: channelConfig(tc.kind, name+"-renamed", tc.reduced, ""),
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(addr, "app_deploy", "false"),
							resource.TestCheckResourceAttr(addr, "database_backup", "false"),
							checkServer(addr, func(n *client.Notification) error {
								if n.Name != name+"-renamed" || n.AppDeploy || n.DatabaseBackup {
									return fmt.Errorf("record = name %q events %+v", n.Name, n.NotificationEvents)
								}
								return tc.checkReduced(n)
							}),
						),
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate)},
							PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
						},
					},
					{
						ResourceName:      addr,
						ImportState:       true,
						ImportStateVerify: true,
					},
				},
			})
		})
	}
}

// TestAccNotification_writeOnlyToken pins the engine's secret path on one
// channel: the state never holds the token, a rename resends the stored
// one, a new version sends the new one, and the plain attribute takes over
// again with an in-place update.
func TestAccNotification_writeOnlyToken(t *testing.T) {
	name := acctest.RandomName("notif-wo")
	addr := "dokploy_telegram_notification.test"
	wo := func(token string, version int) string {
		return fmt.Sprintf("  chat_id              = \"-100\"\n  bot_token_wo         = %q\n  bot_token_wo_version = %d", token, version)
	}
	stored := func(token string) resource.TestCheckFunc {
		return checkServer(addr, func(n *client.Notification) error {
			if n.Telegram.BotToken != token {
				return fmt.Errorf("server bot token = %q, want %q", n.Telegram.BotToken, token)
			}
			return nil
		})
	}
	noSecretInState := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckNoResourceAttr(addr, "bot_token"),
		resource.TestCheckNoResourceAttr(addr, "bot_token_wo"),
	)
	update := resource.ConfigPlanChecks{
		PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate)},
		PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkDestroy,
		Steps: []resource.TestStep{
			{
				Config: channelConfig("telegram", name, wo("tok-1", 1), ""),
				Check:  resource.ComposeAggregateTestCheckFunc(noSecretInState, stored("tok-1")),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config:           channelConfig("telegram", name+"-renamed", wo("tok-1", 1), ""),
				ConfigPlanChecks: update,
				Check:            resource.ComposeAggregateTestCheckFunc(noSecretInState, stored("tok-1")),
			},
			{
				Config:           channelConfig("telegram", name+"-renamed", wo("tok-2", 2), ""),
				ConfigPlanChecks: update,
				Check:            resource.ComposeAggregateTestCheckFunc(noSecretInState, stored("tok-2")),
			},
			{
				Config:           channelConfig("telegram", name+"-renamed", "  chat_id   = \"-100\"\n  bot_token = \"tok-3\"", ""),
				ConfigPlanChecks: update,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "bot_token", "tok-3"),
					stored("tok-3"),
				),
			},
		},
	})
}

var _ = str
