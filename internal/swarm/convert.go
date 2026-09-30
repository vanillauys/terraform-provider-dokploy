package swarm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// wireAcronyms lists the attribute names whose Dokploy key is not the
// PascalCase of the snake case name.
var wireAcronyms = map[string]string{"os": "OS"}

// wireKey maps a snake case attribute name to the Docker key that Dokploy
// stores, for example max_failure_ratio to MaxFailureRatio.
func wireKey(name string) string {
	if key, ok := wireAcronyms[name]; ok {
		return key
	}
	var b strings.Builder
	for _, part := range strings.Split(name, "_") {
		b.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return b.String()
}

// attrTypes returns the attribute types of the swarm object.
func attrTypes() map[string]attr.Type {
	return Attribute().GetType().(basetypes.ObjectType).AttrTypes
}

// columns returns the address of each client column in the order of the
// swarm attribute names. The map key is the attribute name.
func columns(s *client.Swarm) map[string]*json.RawMessage {
	return map[string]*json.RawMessage{
		"health_check":      &s.HealthCheck,
		"restart_policy":    &s.RestartPolicy,
		"placement":         &s.Placement,
		"update_config":     &s.UpdateConfig,
		"rollback_config":   &s.RollbackConfig,
		"mode":              &s.Mode,
		"labels":            &s.Labels,
		"network":           &s.Network,
		"endpoint_spec":     &s.EndpointSpec,
		"ulimits":           &s.Ulimits,
		"stop_grace_period": &s.StopGracePeriod,
	}
}

// toWire converts a Terraform value to the JSON value of a Dokploy column.
// A null attribute inside an object is left out of the object, so a column
// holds exactly the keys that the configuration sets.
func toWire(v attr.Value) any {
	switch v := v.(type) {
	case basetypes.ObjectValue:
		out := map[string]any{}
		for name, av := range v.Attributes() {
			if av.IsNull() || av.IsUnknown() {
				continue
			}
			out[wireKey(name)] = toWire(av)
		}
		return out
	case basetypes.ListValue:
		out := make([]any, 0, len(v.Elements()))
		for _, e := range v.Elements() {
			out = append(out, toWire(e))
		}
		return out
	case basetypes.MapValue:
		out := make(map[string]any, len(v.Elements()))
		for k, e := range v.Elements() {
			out[k] = toWire(e)
		}
		return out
	case basetypes.StringValue:
		return v.ValueString()
	case basetypes.Int64Value:
		return v.ValueInt64()
	case basetypes.Float64Value:
		return v.ValueFloat64()
	}
	return nil
}

// Expand converts the planned swarm block to the eleven client columns. A
// null block, and a null attribute of the block, gives a nil column, which
// marshals to the explicit null that clears the stored value.
func Expand(obj types.Object) (client.Swarm, diag.Diagnostics) {
	var out client.Swarm
	var diags diag.Diagnostics
	if obj.IsNull() || obj.IsUnknown() {
		return out, diags
	}
	cols := columns(&out)
	for name, av := range obj.Attributes() {
		if av.IsNull() || av.IsUnknown() {
			continue
		}
		raw, err := json.Marshal(toWire(av))
		if err != nil {
			diags.AddError("Cannot encode swarm."+name, err.Error())
			continue
		}
		*cols[name] = raw
	}
	return out, diags
}

// fromWire converts the JSON value of a Dokploy column to a Terraform value
// of type t. A missing key and a JSON null give a null value.
func fromWire(v any, t attr.Type) (attr.Value, error) {
	if v == nil {
		return nullOf(t), nil
	}
	switch t := t.(type) {
	case basetypes.ObjectType:
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected an object, got %T", v)
		}
		values := make(map[string]attr.Value, len(t.AttrTypes))
		for name, at := range t.AttrTypes {
			av, err := fromWire(m[wireKey(name)], at)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", wireKey(name), err)
			}
			values[name] = av
		}
		obj, d := types.ObjectValue(t.AttrTypes, values)
		return obj, firstError(d)
	case basetypes.ListType:
		items, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("expected an array, got %T", v)
		}
		values := make([]attr.Value, len(items))
		for i, item := range items {
			av, err := fromWire(item, t.ElemType)
			if err != nil {
				return nil, err
			}
			values[i] = av
		}
		list, d := types.ListValue(t.ElemType, values)
		return list, firstError(d)
	case basetypes.MapType:
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected an object, got %T", v)
		}
		values := make(map[string]attr.Value, len(m))
		for k, item := range m {
			av, err := fromWire(item, t.ElemType)
			if err != nil {
				return nil, err
			}
			values[k] = av
		}
		out, d := types.MapValue(t.ElemType, values)
		return out, firstError(d)
	}
	return scalarFromWire(v, t)
}

func scalarFromWire(v any, t attr.Type) (attr.Value, error) {
	switch {
	case t.Equal(types.StringType):
		if s, ok := v.(string); ok {
			return types.StringValue(s), nil
		}
	case t.Equal(types.Int64Type):
		if n, ok := v.(json.Number); ok {
			i, err := n.Int64()
			return types.Int64Value(i), err
		}
	case t.Equal(types.Float64Type):
		if n, ok := v.(json.Number); ok {
			f, err := n.Float64()
			return types.Float64Value(f), err
		}
	}
	return nil, fmt.Errorf("cannot read %v as %s", v, t)
}

// nullOf returns the null value of type t.
func nullOf(t attr.Type) attr.Value {
	v, err := t.ValueFromTerraform(context.Background(), tftypes.NewValue(t.TerraformType(context.Background()), nil))
	if err != nil {
		panic(err)
	}
	return v
}

func firstError(d diag.Diagnostics) error {
	if d.HasError() {
		return fmt.Errorf("%s", d.Errors()[0].Detail())
	}
	return nil
}

// isNull reports whether a column holds no value: absent, or a JSON null.
func isNull(raw json.RawMessage) bool {
	return len(raw) == 0 || bytes.Equal(raw, []byte("null"))
}

// Flatten converts the eleven client columns to the swarm block. prior is the
// block of the state before the read: a null prior block stays null while
// every column is null, and a block that the configuration wrote as `{}`
// stays a block. A record that holds values outside Terraform always reads
// back as a block, so the next plan shows the drift.
func Flatten(ctx context.Context, s client.Swarm, prior types.Object) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics
	cols := columns(&s)
	empty := true
	values := make(map[string]attr.Value, len(cols))
	for name, at := range attrTypes() {
		raw := *cols[name]
		if !isNull(raw) {
			empty = false
		}
		var decoded any
		if !isNull(raw) {
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.UseNumber()
			if err := dec.Decode(&decoded); err != nil {
				diags.AddError("Cannot decode swarm."+name, err.Error())
				continue
			}
		}
		av, err := fromWire(decoded, at)
		if err != nil {
			diags.AddError("Cannot read swarm."+name, err.Error())
			continue
		}
		values[name] = av
	}
	if diags.HasError() {
		return types.ObjectNull(attrTypes()), diags
	}
	if empty && prior.IsNull() {
		return types.ObjectNull(attrTypes()), diags
	}
	obj, d := types.ObjectValue(attrTypes(), values)
	diags.Append(d...)
	return obj, diags
}
