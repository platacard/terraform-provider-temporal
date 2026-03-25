package provider

import (
	"testing"
	"time"

	schedulev1 "go.temporal.io/api/schedule/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

// TestConvertScheduleSpec_NilSpec verifies that convertScheduleSpec returns nil
// without panicking when called with a nil spec
func TestConvertScheduleSpec_NilSpec(t *testing.T) {
	model := convertScheduleSpec(nil)
	if model != nil {
		t.Errorf("expected nil for nil input, got %+v", model)
	}
}

// TestConvertScheduleSpec_NilJitter verifies that an absent Jitter field results
// in a null Terraform value rather than a panic
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
// text format ("seconds:300 nanos:0"), which caused an infinite plan/apply
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
// returns an error for a nil schedule without panicking
func TestConvertFromTemporalSchedule_NilSchedule(t *testing.T) {
	_, err := ConvertFromTemporalSchedule("id", "ns", nil)
	if err == nil {
		t.Error("expected error for nil schedule, got nil")
	}
}

// TestConvertFromTemporalSchedule_NilSpecAndAction verifies that a schedule with
// nil Spec and Action fields does not panic and leaves the model fields nil
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
