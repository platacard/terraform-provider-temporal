package provider

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	namespace "go.temporal.io/api/namespace/v1"
	"go.temporal.io/api/workflowservice/v1"
	"google.golang.org/protobuf/types/known/durationpb"
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
			ns := &workflowservice.DescribeNamespaceResponse{Config: &namespace.NamespaceConfig{WorkflowExecutionRetentionTtl: durationpb.New(d)}}
			model := NamespaceResourceModel{Retention: types.StringValue(tc.input)}
			if diags := updateModelFromSpec(context.Background(), &model, ns); diags.HasError() {
				t.Fatal(diags)
			}
			if model.Retention.ValueString() != tc.input {
				t.Fatalf("changed configured spelling: %s", model.Retention)
			}
			imported := NamespaceResourceModel{}
			updateModelFromSpec(context.Background(), &imported, ns)
			actual, err := parseRetention(imported.Retention.ValueString())
			if err != nil || actual != d {
				t.Fatalf("import lost precision: %s", imported.Retention)
			}
			ns.Config.WorkflowExecutionRetentionTtl = durationpb.New(d + time.Second)
			updateModelFromSpec(context.Background(), &model, ns)
			actual, _ = parseRetention(model.Retention.ValueString())
			if actual != d+time.Second {
				t.Fatal("remote retention drift not detected")
			}
		})
	}
}

func TestRetentionStateUpgrade(t *testing.T) {
	for _, value := range []string{"3", "0", "null"} {
		t.Run(value, func(t *testing.T) {
			req := resource.UpgradeStateRequest{RawState: &tfprotov6.RawState{JSON: []byte(`{"retention":` + value + `,"name":"keep-me","clusters":["active"]}`)}}
			resp := resource.UpgradeStateResponse{}
			(&NamespaceResource{}).UpgradeState(context.Background())[0].StateUpgrader(context.Background(), req, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			var result map[string]json.RawMessage
			if err := json.Unmarshal(resp.DynamicValue.JSON, &result); err != nil {
				t.Fatal(err)
			}
			expected := value
			if value != "null" {
				expected = `"` + value + `"`
			}
			if string(result["retention"]) != expected || string(result["name"]) != `"keep-me"` || string(result["clusters"]) != `["active"]` {
				t.Fatalf("unexpected state: %s", resp.DynamicValue.JSON)
			}
		})
	}
}

func TestRetentionSemanticEquality(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		prior, planned, want types.String
	}{
		{"default migration", types.StringValue("3"), types.StringValue("72h"), types.StringValue("3")},
		{"import", types.StringValue("1h0m1s"), types.StringValue("1h1s"), types.StringValue("1h0m1s")},
		{"real change", types.StringValue("1h"), types.StringValue("2h"), types.StringValue("2h")},
		{"new resource", types.StringNull(), types.StringValue("72h"), types.StringValue("72h")},
		{"destroy", types.StringValue("3"), types.StringNull(), types.StringNull()},
		{"unknown plan", types.StringValue("3"), types.StringUnknown(), types.StringUnknown()},
		{"unknown prior", types.StringUnknown(), types.StringValue("72h"), types.StringValue("72h")},
		{"invalid prior", types.StringValue("invalid"), types.StringValue("72h"), types.StringValue("72h")},
		{"invalid plan", types.StringValue("3"), types.StringValue("invalid"), types.StringValue("invalid")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := planmodifier.StringRequest{StateValue: tc.prior, PlanValue: tc.planned}
			resp := planmodifier.StringResponse{PlanValue: tc.planned}
			retentionSemanticEquality{}.PlanModifyString(context.Background(), req, &resp)
			if !resp.PlanValue.Equal(tc.want) || resp.Diagnostics.HasError() {
				t.Fatalf("got %s, want %s; diagnostics: %v", resp.PlanValue, tc.want, resp.Diagnostics)
			}
		})
	}
}
