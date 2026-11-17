package provider

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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
