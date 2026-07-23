// Package tfutil holds small schema helpers shared by service resources.
package tfutil

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// DeployAttributes returns the deploy-engine attributes every service
// resource carries (spec §5.5): deploy_on_change default true,
// deployment_timeout default "15m".
func DeployAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"deploy_on_change": schema.BoolAttribute{
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(true),
			Description: "Deploy after create and after changes to deploy-triggering attributes. Defaults to `true`.",
		},
		"deployment_timeout": schema.StringAttribute{
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString("15m"),
			Validators:  []validator.String{DurationString()},
			Description: "How long to wait for a triggered deployment to reach a terminal status, as a Go duration string. Defaults to `\"15m\"`. On timeout the apply fails but the server-side deployment keeps running.",
		},
	}
}

// ParseTimeout parses deployment_timeout, defaulting to 15m on null/unknown.
func ParseTimeout(v types.String) (time.Duration, error) {
	if v.IsNull() || v.IsUnknown() || v.ValueString() == "" {
		return 15 * time.Minute, nil
	}
	return time.ParseDuration(v.ValueString())
}

type durationString struct{}

func DurationString() validator.String { return durationString{} }

func (durationString) Description(context.Context) string {
	return "a Go duration string such as \"15m\" or \"1h30m\""
}

func (d durationString) MarkdownDescription(ctx context.Context) string {
	return d.Description(ctx)
}

func (durationString) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := time.ParseDuration(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid duration", err.Error())
	}
}
