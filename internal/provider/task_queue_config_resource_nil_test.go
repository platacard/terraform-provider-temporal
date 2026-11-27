package provider

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"go.temporal.io/api/enums/v1"
	taskqueuev1 "go.temporal.io/api/taskqueue/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestParseTaskQueueConfigID(t *testing.T) {
	valid := map[string][3]string{
		"default:payments:Activity":     {"default", "payments", "Activity"},
		"default:payments:Workflow":     {"default", "payments", "Workflow"},
		"my-ns:queue:with:colons:Nexus": {"my-ns", "queue:with:colons", "Nexus"},
	}
	for id, want := range valid {
		namespace, taskQueue, taskQueueType, ok := parseTaskQueueConfigID(id)
		if !ok || namespace != want[0] || taskQueue != want[1] || taskQueueType != want[2] {
			t.Errorf("%q: got (%q, %q, %q, %t), want %v", id, namespace, taskQueue, taskQueueType, ok, want)
		}
	}

	for _, id := range []string{"", "payments", "default:payments", "default:payments:Sticky", ":payments:Activity", "default::Activity", "default:payments:"} {
		if _, _, _, ok := parseTaskQueueConfigID(id); ok {
			t.Errorf("%q: expected the ID to be rejected", id)
		}
	}
}

func TestTaskQueueRateLimitUpdate(t *testing.T) {
	current := &taskqueuev1.RateLimitConfig{RateLimit: &taskqueuev1.RateLimit{RequestsPerSecond: 7}}
	cleared := &taskqueuev1.RateLimitConfig{Metadata: &taskqueuev1.ConfigMetadata{Reason: "old"}}
	desired := &TaskQueueRateLimitModel{RequestsPerSecond: types.Float64Value(0), Reason: types.StringValue("paused")}

	update := taskQueueRateLimitUpdate(desired, current)
	if update.GetRateLimit() == nil || update.GetRateLimit().GetRequestsPerSecond() != 0 || update.GetReason() != "paused" {
		t.Errorf("set to zero: unexpected update %v", update)
	}

	update = taskQueueRateLimitUpdate(nil, current)
	if update == nil || update.GetRateLimit() != nil {
		t.Errorf("remove an existing limit: expected an update without a rate limit, got %v", update)
	}

	for name, existing := range map[string]*taskqueuev1.RateLimitConfig{"no limit": nil, "cleared limit": cleared} {
		if update := taskQueueRateLimitUpdate(nil, existing); update != nil {
			t.Errorf("%s: expected no update, got %v", name, update)
		}
	}
}

func TestTaskQueueConfigModel_UpdateRequest(t *testing.T) {
	model := TaskQueueConfigModel{
		Namespace:     types.StringValue("default"),
		TaskQueue:     types.StringValue("payments"),
		TaskQueueType: types.StringValue("Activity"),
	}
	current := &taskqueuev1.TaskQueueConfig{
		QueueRateLimit:          &taskqueuev1.RateLimitConfig{RateLimit: &taskqueuev1.RateLimit{RequestsPerSecond: 7}},
		FairnessWeightOverrides: map[string]float32{"keep": 1, "drop-a": 2, "drop-b": 3},
	}

	request := model.updateRequest(map[string]float32{"keep": 5, "new": 6}, current)

	if request.GetNamespace() != "default" || request.GetTaskQueue() != "payments" || request.GetTaskQueueType() != enums.TASK_QUEUE_TYPE_ACTIVITY {
		t.Errorf("unexpected target: %v", request)
	}
	if request.GetUpdateQueueRateLimit() == nil || request.GetUpdateQueueRateLimit().GetRateLimit() != nil {
		t.Errorf("expected the existing queue rate limit to be removed, got %v", request.GetUpdateQueueRateLimit())
	}
	if request.GetUpdateFairnessKeyRateLimitDefault() != nil {
		t.Errorf("expected no fairness key rate limit update, got %v", request.GetUpdateFairnessKeyRateLimitDefault())
	}
	if len(request.GetSetFairnessWeightOverrides()) != 2 || request.GetSetFairnessWeightOverrides()["keep"] != 5 {
		t.Errorf("unexpected weights to set: %v", request.GetSetFairnessWeightOverrides())
	}
	unset := slices.Sorted(slices.Values(request.GetUnsetFairnessWeightOverrides()))
	if !slices.Equal(unset, []string{"drop-a", "drop-b"}) {
		t.Errorf("unexpected weights to unset: %v", unset)
	}
}

func TestTaskQueueConfigModel_SetFromConfig(t *testing.T) {
	model := TaskQueueConfigModel{
		QueueRateLimit: &TaskQueueRateLimitModel{RequestsPerSecond: types.Float64Value(1)},
	}

	if diags := model.setFromConfig(context.Background(), nil); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if model.QueueRateLimit != nil || model.FairnessKeyRateLimitDefault != nil {
		t.Errorf("empty configuration: expected no rate limits, got %+v", model)
	}
	if model.FairnessWeightOverrides.IsNull() || len(model.FairnessWeightOverrides.Elements()) != 0 {
		t.Errorf("empty configuration: expected an empty map, got %s", model.FairnessWeightOverrides)
	}

	diags := model.setFromConfig(context.Background(), &taskqueuev1.TaskQueueConfig{
		QueueRateLimit: &taskqueuev1.RateLimitConfig{
			RateLimit: &taskqueuev1.RateLimit{},
			Metadata:  &taskqueuev1.ConfigMetadata{Reason: "paused"},
		},
		FairnessKeysRateLimitDefault: &taskqueuev1.RateLimitConfig{Metadata: &taskqueuev1.ConfigMetadata{}},
		FairnessWeightOverrides:      map[string]float32{"tenant-a": 2.5},
	})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if model.QueueRateLimit == nil || model.QueueRateLimit.RequestsPerSecond.ValueFloat64() != 0 || model.QueueRateLimit.Reason.ValueString() != "paused" {
		t.Errorf("expected a zero queue rate limit, got %+v", model.QueueRateLimit)
	}
	if model.FairnessKeyRateLimitDefault != nil {
		t.Errorf("a cleared limit must read as unset, got %+v", model.FairnessKeyRateLimitDefault)
	}
	weights := map[string]float64{}
	if d := model.FairnessWeightOverrides.ElementsAs(context.Background(), &weights, false); d.HasError() || weights["tenant-a"] != 2.5 {
		t.Errorf("unexpected weights %v (diagnostics: %v)", weights, d)
	}
}

func TestFloat32ToFloat64(t *testing.T) {
	cases := map[float32]float64{
		0:     0,
		0.1:   0.1,
		0.3:   0.3,
		0.7:   0.7,
		0.001: 0.001,
		3.5:   3.5,
		25:    25,
		1000:  1000,
	}
	for input, want := range cases {
		if got := float32ToFloat64(input); got != want {
			t.Errorf("float32ToFloat64(%v): got %v, want %v", input, got, want)
		}
	}
}

func TestTaskQueueConfigModel_ErrorMessage(t *testing.T) {
	model := TaskQueueConfigModel{
		Namespace:     types.StringValue("default"),
		TaskQueue:     types.StringValue("payments"),
		TaskQueueType: types.StringValue("Activity"),
	}
	const hint = "requires Temporal Server 1.29 or later"

	unsupported := model.errorMessage("update", status.Error(codes.Unimplemented, "unknown method UpdateTaskQueueConfig"))
	if !strings.Contains(unsupported, "Activity task queue payments in namespace default") || !strings.Contains(unsupported, "unknown method UpdateTaskQueueConfig") {
		t.Errorf("the message must name the task queue and keep the server error: %q", unsupported)
	}
	if !strings.Contains(unsupported, hint) {
		t.Errorf("expected the server version hint for an unimplemented call: %q", unsupported)
	}

	for _, err := range []error{status.Error(codes.InvalidArgument, "bad"), errors.New("boom")} {
		if message := model.errorMessage("update", err); strings.Contains(message, hint) {
			t.Errorf("unexpected server version hint for %v: %q", err, message)
		}
	}
}
