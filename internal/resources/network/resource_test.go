package network

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// TestFlattenIPAM pins the {}-collapse flattenIPAM's own doc comment
// describes: both nil and Docker's "empty shape" ({} - a zero-value
// NetworkIPAM with no driver and no config) must read back as a null ipam
// object, or a UI-imported network with no custom IPAM would diff forever
// against a config that omits ipam. A populated value must round-trip, and
// an inner field the server echoes as "" must flatten to null rather than
// an empty string, matching an omitted config value.
func TestFlattenIPAM(t *testing.T) {
	ctx := context.Background()

	t.Run("nil collapses to null", func(t *testing.T) {
		var diags diag.Diagnostics
		got := flattenIPAM(ctx, nil, &diags)
		if diags.HasError() {
			t.Fatalf("diags = %v", diags)
		}
		if !got.IsNull() {
			t.Errorf("flattenIPAM(nil) = %v, want null", got)
		}
	})

	t.Run("the empty shape collapses to null", func(t *testing.T) {
		var diags diag.Diagnostics
		got := flattenIPAM(ctx, &client.NetworkIPAM{}, &diags)
		if diags.HasError() {
			t.Fatalf("diags = %v", diags)
		}
		if !got.IsNull() {
			t.Errorf("flattenIPAM(&NetworkIPAM{}) = %v, want null", got)
		}
	})

	t.Run("a populated value round-trips", func(t *testing.T) {
		var diags diag.Diagnostics
		ipam := &client.NetworkIPAM{
			Driver: "default",
			Config: []client.NetworkIPAMConfig{
				{Subnet: "172.28.0.0/16", Gateway: "172.28.0.1", IPRange: "172.28.5.0/24"},
			},
		}
		got := flattenIPAM(ctx, ipam, &diags)
		if diags.HasError() {
			t.Fatalf("diags = %v", diags)
		}
		if got.IsNull() || got.IsUnknown() {
			t.Fatalf("flattenIPAM(populated) = %v, want a value", got)
		}

		var m ipamModel
		diags.Append(got.As(ctx, &m, basetypes.ObjectAsOptions{})...)
		if diags.HasError() {
			t.Fatalf("As: %v", diags)
		}
		if m.Driver.ValueString() != "default" {
			t.Errorf("driver = %q, want default", m.Driver.ValueString())
		}

		var cfgs []ipamConfigModel
		diags.Append(m.Config.ElementsAs(ctx, &cfgs, false)...)
		if diags.HasError() {
			t.Fatalf("ElementsAs: %v", diags)
		}
		if len(cfgs) != 1 {
			t.Fatalf("config = %+v, want 1 entry", cfgs)
		}
		c := cfgs[0]
		if c.Subnet.ValueString() != "172.28.0.0/16" ||
			c.Gateway.ValueString() != "172.28.0.1" ||
			c.IPRange.ValueString() != "172.28.5.0/24" {
			t.Errorf("config[0] = %+v", c)
		}
	})

	t.Run("an inner empty string collapses to null, not an empty value", func(t *testing.T) {
		var diags diag.Diagnostics
		ipam := &client.NetworkIPAM{
			Driver: "default",
			Config: []client.NetworkIPAMConfig{{Subnet: "172.28.0.0/16"}},
		}
		got := flattenIPAM(ctx, ipam, &diags)
		if diags.HasError() {
			t.Fatalf("diags = %v", diags)
		}

		var m ipamModel
		diags.Append(got.As(ctx, &m, basetypes.ObjectAsOptions{})...)
		var cfgs []ipamConfigModel
		diags.Append(m.Config.ElementsAs(ctx, &cfgs, false)...)
		if diags.HasError() {
			t.Fatalf("diags = %v", diags)
		}
		if len(cfgs) != 1 {
			t.Fatalf("config = %+v, want 1 entry", cfgs)
		}
		c := cfgs[0]
		if c.Subnet.ValueString() != "172.28.0.0/16" {
			t.Errorf("subnet = %q, want 172.28.0.0/16", c.Subnet.ValueString())
		}
		if !c.Gateway.IsNull() {
			t.Errorf("gateway = %v, want null when the server echoes \"\"", c.Gateway)
		}
		if !c.IPRange.IsNull() {
			t.Errorf("ip_range = %v, want null when the server echoes \"\"", c.IPRange)
		}
	})
}
