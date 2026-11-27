package provider

import (
	"context"
	"testing"
	"time"

	commonv1 "go.temporal.io/api/common/v1"
	schedulev1 "go.temporal.io/api/schedule/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestScheduleMemoToMap_NilMemo(t *testing.T) {
	result, diags := scheduleMemoToMap(context.Background(), nil)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if result.IsNull() || result.IsUnknown() {
		t.Fatalf("expected an empty map, got %s", result)
	}
	if len(result.Elements()) != 0 {
		t.Errorf("expected no elements, got %d", len(result.Elements()))
	}
}

func TestScheduleMemoToMap_Values(t *testing.T) {
	memo := &commonv1.Memo{Fields: map[string]*commonv1.Payload{
		"text":   {Data: []byte(`"plain"`)},
		"number": {Data: []byte(`42`)},
		"object": {Data: []byte(`{"a":1}`)},
		"raw":    {Data: []byte(`not json`)},
		"empty":  {},
		"binary": {Data: []byte{0xff, 0xfe}},
	}}

	result, diags := scheduleMemoToMap(context.Background(), memo)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if diags.WarningsCount() != 1 {
		t.Errorf("expected one warning for the binary value, got %d", diags.WarningsCount())
	}

	got := map[string]string{}
	if d := result.ElementsAs(context.Background(), &got, false); d.HasError() {
		t.Fatalf("unexpected diagnostics: %v", d)
	}
	want := map[string]string{
		"text":   "plain",
		"number": "42",
		"object": `{"a":1}`,
		"raw":    "not json",
		"empty":  "",
		"binary": "//4=",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d elements, want %d: %v", len(got), len(want), got)
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("memo[%q]: got %q, want %q", key, got[key], value)
		}
	}
}

func TestScheduleDataSourceSpec_EmptyLists(t *testing.T) {
	if model := scheduleDataSourceSpec(nil); model != nil {
		t.Errorf("nil spec: expected nil, got %+v", model)
	}

	calendarOnly := scheduleDataSourceSpec(&schedulev1.ScheduleSpec{
		StructuredCalendar: []*schedulev1.StructuredCalendarSpec{{Hour: []*schedulev1.Range{{Start: 12}}}},
	})
	if calendarOnly.Intervals == nil || len(calendarOnly.Intervals) != 0 {
		t.Errorf("calendar-only spec: expected empty, non-nil intervals, got %#v", calendarOnly.Intervals)
	}
	if calendarOnly.CronItems == nil || len(calendarOnly.CronItems) != 0 {
		t.Errorf("calendar-only spec: expected empty, non-nil cron items, got %#v", calendarOnly.CronItems)
	}
	if len(calendarOnly.CalendarItems) != 1 {
		t.Errorf("calendar-only spec: expected one calendar item, got %#v", calendarOnly.CalendarItems)
	}

	intervalOnly := scheduleDataSourceSpec(&schedulev1.ScheduleSpec{
		Interval: []*schedulev1.IntervalSpec{{Interval: durationpb.New(time.Hour)}},
	})
	if intervalOnly.CalendarItems == nil || len(intervalOnly.CalendarItems) != 0 {
		t.Errorf("interval-only spec: expected empty, non-nil calendar items, got %#v", intervalOnly.CalendarItems)
	}
	if len(intervalOnly.Intervals) != 1 {
		t.Errorf("interval-only spec: expected one interval, got %#v", intervalOnly.Intervals)
	}
}
