package provider

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestRetentionRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		input    string
		duration time.Duration
	}{
		{"1h", time.Hour}, {"1h1s", time.Hour + time.Second},
		{"90m30s", 90*time.Minute + 30*time.Second},
		{"3", 72 * time.Hour}, {"1h0.5s", time.Hour + 500*time.Millisecond},
	} {
		t.Run(tc.input, func(t *testing.T) {
			d, err := parseRetention(tc.input)
			if err != nil || d != tc.duration {
				t.Fatalf("got %v, %v", d, err)
			}
		})
	}
}

func TestRetentionSemanticEquality(t *testing.T) {
	for _, tc := range []struct{ name, prior, planned, want string }{
		{"same duration", "1h0m1s", "1h1s", "1h0m1s"},
		{"different duration", "1h", "2h", "2h"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := planmodifier.StringRequest{StateValue: types.StringValue(tc.prior), PlanValue: types.StringValue(tc.planned)}
			resp := planmodifier.StringResponse{PlanValue: req.PlanValue}
			retentionSemanticEquality{}.PlanModifyString(context.Background(), req, &resp)
			if resp.PlanValue.ValueString() != tc.want {
				t.Fatalf("got %s, want %s", resp.PlanValue, tc.want)
			}
		})
	}
}
