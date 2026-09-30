package notification_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/vanillauys/terraform-provider-dokploy/internal/acctest"
)

// verifyCase is one channel for the verify_connection tests. The rig cannot
// supply a working webhook, SMTP server, or token, so a channel whose test
// endpoint reports failures gets only the failure path: its attributes point
// at 127.0.0.1:9, where nothing listens. A channel with passes set is one
// whose Dokploy v0.30.8 test endpoint reports success for any input (probed
// live, 2026-09-30: telegram and lark), so it gets the pass path only.
type verifyCase struct {
	kind   string
	label  string
	attrs  string
	passes bool
}

var verifyCases = []verifyCase{
	{kind: "slack", label: "Slack", attrs: `  webhook_url = "http://127.0.0.1:9/x"`},
	{kind: "discord", label: "Discord", attrs: `  webhook_url = "http://127.0.0.1:9/x"`},
	{kind: "telegram", label: "Telegram", attrs: "  bot_token = \"123:abc\"\n  chat_id   = \"-100123\"", passes: true},
	{kind: "email", label: "Email", attrs: "  smtp_server  = \"127.0.0.1\"\n  smtp_port    = 9\n  username     = \"u\"\n  password     = \"p\"\n  from_address = \"a@example.com\"\n  to_addresses = [\"b@example.com\"]"},
	{kind: "resend", label: "Resend", attrs: "  api_key      = \"re_invalid\"\n  from_address = \"a@example.com\"\n  to_addresses = [\"b@example.com\"]"},
	{kind: "gotify", label: "Gotify", attrs: "  server_url = \"http://127.0.0.1:9\"\n  app_token  = \"t\""},
	{kind: "ntfy", label: "ntfy", attrs: "  server_url = \"http://127.0.0.1:9\"\n  topic      = \"t\""},
	{kind: "mattermost", label: "Mattermost", attrs: `  webhook_url = "http://127.0.0.1:9/x"`},
	{kind: "lark", label: "Lark", attrs: `  webhook_url = "http://127.0.0.1:9/x"`, passes: true},
	{kind: "teams", label: "Microsoft Teams", attrs: `  webhook_url = "http://127.0.0.1:9/x"`},
	{kind: "pushover", label: "Pushover", attrs: "  user_key  = \"uk\"\n  api_token = \"at\""},
	{kind: "custom", label: "Custom webhook", attrs: `  endpoint = "http://127.0.0.1:9/x"`},
}

// TestAccNotification_verifyConnection covers the twelve channels through
// the one shared engine. A failing test must abort the create and the update
// with the server message, and leave the stored record as it was. A passing
// test must keep the plan empty. Import must seed the attribute with false.
func TestAccNotification_verifyConnection(t *testing.T) {
	for _, tc := range verifyCases {
		t.Run(tc.kind, func(t *testing.T) {
			addr := "dokploy_" + tc.kind + "_notification.test"
			name := acctest.RandomName("notif-v-" + tc.kind)
			verify := "  verify_connection = true"
			failure := regexp.MustCompile(fmt.Sprintf("Verifying %s notification connection", tc.label))
			noDiff := resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}
			steps := []resource.TestStep{
				{
					Config:      channelConfig(tc.kind, name, tc.attrs, verify),
					ExpectError: failure,
				},
				{
					Config:           channelConfig(tc.kind, name, tc.attrs, ""),
					Check:            resource.TestCheckNoResourceAttr(addr, "verify_connection"),
					ConfigPlanChecks: noDiff,
				},
				{
					Config:      channelConfig(tc.kind, name+"-renamed", tc.attrs, verify),
					ExpectError: failure,
				},
				{
					ResourceName:      addr,
					ImportState:       true,
					ImportStateVerify: true,
				},
			}
			if tc.passes {
				steps = []resource.TestStep{
					{
						Config:           channelConfig(tc.kind, name, tc.attrs, verify),
						Check:            resource.TestCheckResourceAttr(addr, "verify_connection", "true"),
						ConfigPlanChecks: noDiff,
					},
					{
						Config:           channelConfig(tc.kind, name+"-renamed", tc.attrs, verify),
						Check:            resource.TestCheckResourceAttr(addr, "name", name+"-renamed"),
						ConfigPlanChecks: noDiff,
					},
					{
						Config:           channelConfig(tc.kind, name+"-renamed", tc.attrs, ""),
						Check:            resource.TestCheckNoResourceAttr(addr, "verify_connection"),
						ConfigPlanChecks: noDiff,
					},
					{
						ResourceName:      addr,
						ImportState:       true,
						ImportStateVerify: true,
					},
				}
			}
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { acctest.PreCheck(t) },
				ProtoV6ProviderFactories: acctest.ProviderFactories(),
				CheckDestroy:             checkDestroy,
				Steps:                    steps,
			})
		})
	}
}
