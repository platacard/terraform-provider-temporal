package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

const retentionDescription = "Workflow Execution retention as a duration string, for example 1h, 1h1s, 90m30s, or 72h. Bare integers are accepted as days for compatibility. Temporal enforces a minimum of 1h for local namespaces and 24h for global namespaces."

// Keep accepting legacy integer-day configurations, which Terraform converts to strings.
func parseRetention(value string) (time.Duration, error) {
	if days, err := strconv.ParseInt(value, 10, 64); err == nil {
		if days < 0 || days > int64((1<<63-1)/day) {
			return 0, fmt.Errorf("retention days out of range: %q", value)
		}
		return time.Duration(days) * day, nil
	}
	return time.ParseDuration(value)
}

type retentionValidator struct{}

func (retentionValidator) Description(context.Context) string {
	return "Retention must be a duration of at least 1h, or an integer number of days."
}
func (v retentionValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (v retentionValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	d, err := parseRetention(req.ConfigValue.ValueString())
	if err != nil || d < time.Hour {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid retention", v.Description(ctx))
	}
}

var _ resource.ResourceWithUpgradeState = &NamespaceResource{}

// Upgrade only retention, preserving all other state fields verbatim.
func (r *NamespaceResource) UpgradeState(context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{0: {
		StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
			var state map[string]json.RawMessage
			if req.RawState == nil {
				resp.Diagnostics.AddError("Unable to upgrade namespace state", "Missing prior state")
				return
			}
			if err := json.Unmarshal(req.RawState.JSON, &state); err != nil {
				resp.Diagnostics.AddError("Unable to upgrade namespace state", err.Error())
				return
			}
			if raw, ok := state["retention"]; ok && string(raw) != "null" {
				var days int64
				if err := json.Unmarshal(raw, &days); err != nil {
					resp.Diagnostics.AddError("Unable to upgrade retention", err.Error())
					return
				}
				// Preserve the old numeric spelling so unchanged configurations remain stable.
				state["retention"], _ = json.Marshal(strconv.FormatInt(days, 10))
			}
			data, err := json.Marshal(state)
			if err != nil {
				resp.Diagnostics.AddError("Unable to upgrade namespace state", err.Error())
				return
			}
			resp.DynamicValue = &tfprotov6.DynamicValue{JSON: data}
		},
	}}
}

// Terraform permits retaining the prior state value when it is semantically
// equivalent to configuration. Never construct a third spelling in the plan.
type retentionSemanticEquality struct{}

func (retentionSemanticEquality) Description(context.Context) string {
	return "Keeps the prior retention spelling when it denotes the same duration."
}
func (m retentionSemanticEquality) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}
func (retentionSemanticEquality) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() || req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}
	planned, err := parseRetention(req.PlanValue.ValueString())
	if err != nil {
		return
	}
	prior, err := parseRetention(req.StateValue.ValueString())
	if err != nil {
		return
	}
	if planned == prior {
		resp.PlanValue = req.StateValue
	}
}
