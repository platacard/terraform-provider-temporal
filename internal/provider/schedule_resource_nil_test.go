package provider

import (
	"testing"
	"time"

	commonv1 "go.temporal.io/api/common/v1"
	schedulev1 "go.temporal.io/api/schedule/v1"
	taskqueuev1 "go.temporal.io/api/taskqueue/v1"
	workflowv1 "go.temporal.io/api/workflow/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

// TestConvertScheduleSpec_NilSpec verifies that convertScheduleSpec returns nil
// without panicking when called with a nil spec.
func TestConvertScheduleSpec_NilSpec(t *testing.T) {
	model := convertScheduleSpec(nil)
	if model != nil {
		t.Errorf("expected nil for nil input, got %+v", model)
	}
}

// TestConvertScheduleSpec_NilJitter verifies that an absent Jitter field results
// in a null Terraform value rather than a panic.
func TestConvertScheduleSpec_NilJitter(t *testing.T) {
	spec := &schedulev1.ScheduleSpec{TimezoneName: "UTC"}
	model := convertScheduleSpec(spec)
	if model == nil {
		t.Fatal("expected non-nil model")
	}
	if !model.Jitter.IsNull() {
		t.Errorf("expected Jitter to be null when not set, got %q", model.Jitter.ValueString())
	}
}

// TestConvertScheduleSpec_JitterRoundtrip verifies that the Jitter field is
// serialized as a human-readable duration string ("5m") rather than the protobuf
// text format ("seconds:300 nanos:0"), which caused an infinite plan/apply.
func TestConvertScheduleSpec_JitterRoundtrip(t *testing.T) {
	spec := &schedulev1.ScheduleSpec{
		Jitter: durationpb.New(5 * time.Minute),
	}

	model := convertScheduleSpec(spec)
	if model == nil {
		t.Fatal("expected non-nil ScheduleSpecModel")
	}

	got := model.Jitter.ValueString()
	want := "5m"
	if got != want {
		t.Errorf("Jitter: got %q, want %q — would cause infinite plan/apply drift", got, want)
	}
}

// TestConvertFromTemporalSchedule_NilSchedule verifies that ConvertFromTemporalSchedule
// returns an error for a nil schedule without panicking.
func TestConvertFromTemporalSchedule_NilSchedule(t *testing.T) {
	_, err := ConvertFromTemporalSchedule("id", "ns", nil)
	if err == nil {
		t.Error("expected error for nil schedule, got nil")
	}
}

// TestConvertFromTemporalSchedule_NilSpecAndAction verifies that a schedule with
// nil Spec and Action fields does not panic and leaves the model fields nil.
func TestConvertFromTemporalSchedule_NilSpecAndAction(t *testing.T) {
	sched := &schedulev1.Schedule{}
	model, err := ConvertFromTemporalSchedule("id", "default", sched)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.Spec != nil {
		t.Error("expected Spec to be nil when Schedule.Spec is nil")
	}
	if model.Action != nil {
		t.Error("expected Action to be nil when Schedule.Action is nil")
	}
}

func TestConvertScheduleAction_NilTaskQueue(t *testing.T) {
	action := &schedulev1.ScheduleAction{Action: &schedulev1.ScheduleAction_StartWorkflow{
		StartWorkflow: &workflowv1.NewWorkflowExecutionInfo{WorkflowId: "wf"},
	}}

	model, diags := convertScheduleAction(action)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if model == nil || model.Workflow == nil {
		t.Fatal("expected a workflow action")
	}
	if got := model.Workflow.TaskQueue.ValueString(); got != "" {
		t.Errorf("expected an empty task queue, got %q", got)
	}
	if !model.Workflow.Input.IsNull() {
		t.Errorf("expected a null input, got %s", model.Workflow.Input)
	}
}

func TestConvertScheduleAction_Input(t *testing.T) {
	newAction := func(payloads ...*commonv1.Payload) *schedulev1.ScheduleAction {
		return &schedulev1.ScheduleAction{Action: &schedulev1.ScheduleAction_StartWorkflow{
			StartWorkflow: &workflowv1.NewWorkflowExecutionInfo{
				WorkflowId: "wf",
				TaskQueue:  &taskqueuev1.TaskQueue{Name: "queue"},
				Input:      &commonv1.Payloads{Payloads: payloads},
			},
		}}
	}

	cases := []struct {
		name     string
		action   *schedulev1.ScheduleAction
		want     string
		warnings int
	}{
		{"one JSON argument", newAction(&commonv1.Payload{Data: []byte(`{"key":"value"}`)}), `{"key":"value"}`, 0},
		{"several arguments", newAction(&commonv1.Payload{Data: []byte(`"acct-1"`)}, &commonv1.Payload{Data: []byte(`42`)}), `"acct-1"`, 1},
		{"binary argument", newAction(&commonv1.Payload{Data: []byte{0xff, 0xfe}}), "//4=", 1},
		{"several arguments, the first one binary", newAction(&commonv1.Payload{Data: []byte{0xff, 0xfe}}, &commonv1.Payload{Data: []byte(`42`)}), "//4=", 2},
	}
	for _, c := range cases {
		model, diags := convertScheduleAction(c.action)
		if diags.HasError() {
			t.Errorf("%s: unexpected error: %v", c.name, diags)
		}
		if diags.WarningsCount() != c.warnings {
			t.Errorf("%s: got %d warnings, want %d: %v", c.name, diags.WarningsCount(), c.warnings, diags)
		}
		if got := model.Workflow.Input.ValueString(); got != c.want {
			t.Errorf("%s: got input %q, want %q", c.name, got, c.want)
		}
	}

	model, diags := convertScheduleAction(newAction())
	if len(diags) != 0 || !model.Workflow.Input.IsNull() {
		t.Errorf("no arguments: got input %s with diagnostics %v, want null", model.Workflow.Input, diags)
	}
}
