package dsutil_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// channel is one notification channel: the resource body without the name,
// the attributes that the data source must report, and the secret
// attributes that it must not report.
type channel struct {
	kind    string
	body    string
	attrs   []string
	secrets []string
}

var channels = []channel{
	{"slack", `webhook_url = "https://hooks.slack.test/acc"
  channel     = "#acc"`, []string{"channel"}, []string{"webhook_url"}},
	{"discord", `webhook_url = "https://discord.test/acc"
  decoration  = false`, []string{"decoration"}, []string{"webhook_url"}},
	{"telegram", `bot_token = "acc-bot-token"
  chat_id   = "12345"`, []string{"chat_id"}, []string{"bot_token"}},
	{"email", `smtp_server  = "smtp.example.test"
  smtp_port    = 587
  username     = "acc-user"
  password     = "acc-password"
  from_address = "from@example.test"
  to_addresses = ["to@example.test"]`,
		[]string{"smtp_server", "smtp_port", "username", "from_address", "to_addresses.#"}, []string{"password"}},
	{"resend", `api_key      = "re_acc_key"
  from_address = "from@example.test"
  to_addresses = ["to@example.test"]`, []string{"from_address", "to_addresses.#"}, []string{"api_key"}},
	{"gotify", `server_url = "https://gotify.example.test"
  app_token  = "acc-app-token"
  priority   = 7`, []string{"server_url", "priority", "decoration"}, []string{"app_token"}},
	{"ntfy", `server_url   = "https://ntfy.example.test"
  topic        = "acc"
  access_token = "tk_acc_token"`, []string{"server_url", "topic", "priority"}, []string{"access_token"}},
	{"mattermost", `webhook_url = "https://mattermost.example.test/hooks/acc"
  channel     = "acc"
  username    = "acc-bot"`, []string{"channel", "username"}, []string{"webhook_url"}},
	{"lark", `webhook_url = "https://lark.example.test/hook/acc"`, nil, []string{"webhook_url"}},
	{"teams", `webhook_url = "https://teams.example.test/hook/acc"`, nil, []string{"webhook_url"}},
	{"pushover", `user_key  = "acc-user-key"
  api_token = "acc-api-token"
  priority  = 1`, []string{"priority", "retry", "expire"}, []string{"user_key", "api_token"}},
	{"custom", `endpoint = "https://custom.example.test/hook?token=acc"
  headers  = { Authorization = "Bearer acc" }`, nil, []string{"endpoint", "headers"}},
}

func (c channel) resourceType() string { return "dokploy_" + c.kind + "_notification" }

// config renders the resource of the channel, named after the channel.
func (c channel) config(name string) string {
	return fmt.Sprintf("\nresource %q %q {\n  name         = %q\n  app_deploy   = true\n  %s\n}\n",
		c.resourceType(), "test", name+"-"+c.kind, c.body)
}

func TestAccNotificationDataSources_lookups(t *testing.T) {
	name := acctest.RandomName("ds-notif")
	var resources, lookups strings.Builder
	var checks []resource.TestCheckFunc
	for _, c := range channels {
		resources.WriteString(c.config(name))
		fmt.Fprintf(&lookups, "\ndata %q \"by_id\" {\n  id = %s.test.id\n}\n", c.resourceType(), c.resourceType())
		fmt.Fprintf(&lookups, "\ndata %q \"by_name\" {\n  name = %s.test.name\n}\n", c.resourceType(), c.resourceType())
		for _, lookup := range []string{"by_id", "by_name"} {
			addr := "data." + c.resourceType() + "." + lookup
			attrs := append([]string{"id", "name", "app_deploy", "app_build_error", "server_threshold", "created_at"}, c.attrs...)
			checks = append(checks, matches(addr, c.resourceType()+".test", attrs...))
			for _, secret := range c.secrets {
				checks = append(checks, resource.TestCheckNoResourceAttr(addr, secret))
			}
		}
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: resources.String()},
			{Config: resources.String() + lookups.String(), Check: resource.ComposeAggregateTestCheckFunc(checks...)},
		},
	})
}

func TestAccNotificationDataSources_errors(t *testing.T) {
	name := acctest.RandomName("ds-notif-err")
	cfg := `
resource "dokploy_slack_notification" "first" {
  name        = "` + name + `-twin"
  webhook_url = "https://hooks.slack.test/first"
}

resource "dokploy_slack_notification" "second" {
  name        = "` + name + `-twin"
  webhook_url = "https://hooks.slack.test/second"
  depends_on  = [dokploy_slack_notification.first]
}

resource "dokploy_discord_notification" "other" {
  name        = "` + name + `-other"
  webhook_url = "https://discord.test/other"
}

resource "dokploy_ai" "first" {
  name     = "` + name + `-twin"
  api_url  = "https://api.openai.test/v1"
  api_key  = "sk-acceptance-only"
  model    = "gpt-acc"
}

resource "dokploy_ai" "second" {
  name       = "` + name + `-twin"
  api_url    = "https://api.openai.test/v1"
  api_key    = "sk-acceptance-only-2"
  model      = "gpt-acc"
  depends_on = [dokploy_ai.first]
}

resource "dokploy_ai" "solo" {
  name     = "` + name + `-solo"
  api_url  = "https://api.openai.test/v1"
  api_key  = "sk-acceptance-only-3"
  model    = "gpt-solo"
  depends_on = [dokploy_ai.second]
}
`
	cases := []struct{ name, data, want string }{
		{"slack ambiguous", `data "dokploy_slack_notification" "x" {
  name = "` + name + `-twin"
}`, `more than one .* matches`},
		{"slack missing", `data "dokploy_slack_notification" "x" {
  name = "none"
}`, `no .* matches`},
		{"slack name of another channel", `data "dokploy_slack_notification" "x" {
  name = "` + name + `-other"
}`, `no .* matches`},
		{"slack id of another channel", `data "dokploy_slack_notification" "x" {
  id = dokploy_discord_notification.other.id
}`, `is a discord channel, not slack`},
		{"slack unknown id", `data "dokploy_slack_notification" "x" {
  id = "does-not-exist"
}`, `(?i)not found`},
		{"ai ambiguous", `data "dokploy_ai" "x" {
  name = "` + name + `-twin"
}`, `more than one .* matches`},
		{"ai missing", `data "dokploy_ai" "x" {
  name = "none"
}`, `no .* matches`},
		{"ai unknown id", `data "dokploy_ai" "x" {
  id = "does-not-exist"
}`, `(?i)not found`},
	}
	steps := []resource.TestStep{
		{Config: cfg},
		{
			Config: cfg + `
data "dokploy_ai" "by_id" {
  id = dokploy_ai.solo.id
}

data "dokploy_ai" "by_name" {
  name = dokploy_ai.solo.name
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				matches("data.dokploy_ai.by_id", "dokploy_ai.solo", "id", "name", "api_url", "model", "is_enabled", "organization_id", "created_at"),
				matches("data.dokploy_ai.by_name", "dokploy_ai.solo", "id", "name", "api_url", "model", "is_enabled", "organization_id", "created_at"),
				resource.TestCheckNoResourceAttr("data.dokploy_ai.by_id", "api_key"),
				resource.TestCheckNoResourceAttr("data.dokploy_ai.by_name", "api_key"),
			),
		},
	}
	for _, c := range cases {
		steps = append(steps, resource.TestStep{Config: cfg + c.data, ExpectError: regexp.MustCompile(c.want)})
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps:                    steps,
	})
}
