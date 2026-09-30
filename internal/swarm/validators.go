package swarm

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ReplicasConflict rejects a configuration that sets both the top-level
// `replicas` attribute and `swarm.mode`. Dokploy uses the mode and ignores the
// replicas column when the mode is set (doc.go, "v1.8.0 records"), so the pair
// would hide one of the two values.
func ReplicasConflict() resource.ConfigValidator {
	return resourcevalidator.Conflicting(
		path.MatchRoot("replicas"),
		path.MatchRoot("swarm").AtName("mode"),
	)
}

// ulimitsRejected is the config validator that RejectUlimits returns.
type ulimitsRejected struct{}

func (ulimitsRejected) Description(context.Context) string {
	return "swarm.ulimits is not available on this resource"
}

func (v ulimitsRejected) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (ulimitsRejected) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var ulimits types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("swarm").AtName("ulimits"), &ulimits)...)
	if resp.Diagnostics.HasError() || ulimits.IsNull() {
		return
	}
	resp.Diagnostics.AddAttributeError(path.Root("swarm").AtName("ulimits"),
		"Unsupported swarm attribute",
		"Dokploy has no ulimitsSwarm column for this service type, so the server would ignore the value and every plan would show a change. Remove the `ulimits` attribute.")
}

// RejectUlimits rejects `swarm.ulimits`. The libsql endpoints of Dokploy
// v0.30.8 carry ten of the eleven swarm columns and have no ulimitsSwarm.
func RejectUlimits() resource.ConfigValidator { return ulimitsRejected{} }
