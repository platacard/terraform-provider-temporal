package provider

import (
	"testing"
	"time"

	schedulev1 "go.temporal.io/api/schedule/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestFormatDurationCanonical(t *testing.T) {
	if got := formatDurationCanonical(nil); got != "" {
		t.Errorf("nil duration: got %q, want an empty string", got)
	}

	cases := map[time.Duration]string{
		0:                    "0s",
		61 * time.Second:     "61s",
		time.Minute:          "1m",
		90 * time.Minute:     "90m",
		time.Hour:            "1h",
		24 * time.Hour:       "24h",
		48 * time.Hour:       "48h",
		7 * 24 * time.Hour:   "168h",
		365 * 24 * time.Hour: "8760h",
	}
	for duration, want := range cases {
		got := formatDurationCanonical(durationpb.New(duration))
		if got != want {
			t.Errorf("%s: got %q, want %q", duration, got, want)
		}
		parsed, err := time.ParseDuration(got)
		if err != nil {
			t.Errorf("%s: %q is not a valid duration string: %v", duration, got, err)
		} else if parsed != duration {
			t.Errorf("%s: %q parses back to %s", duration, got, parsed)
		}
	}
}

func TestFormatDurationCanonical_SubSecond(t *testing.T) {
	cases := map[time.Duration]string{
		500 * time.Millisecond:              "500ms",
		1500 * time.Millisecond:             "1.5s",
		90*time.Second + time.Millisecond/2: "1m30.0005s",
		time.Nanosecond:                     "1ns",
	}
	for duration, want := range cases {
		got := formatDurationCanonical(durationpb.New(duration))
		if got != want {
			t.Errorf("%s: got %q, want %q", duration, got, want)
		}
		if parsed, err := time.ParseDuration(got); err != nil || parsed != duration {
			t.Errorf("%s: %q does not parse back to the same duration (%s, %v)", duration, got, parsed, err)
		}
	}
}

func TestFormatRanges(t *testing.T) {
	cases := []struct {
		name   string
		ranges []*schedulev1.Range
		want   string
	}{
		{"no ranges", nil, ""},
		{"single value", []*schedulev1.Range{{Start: 5}}, "5"},
		{"single value with end equal to start", []*schedulev1.Range{{Start: 5, End: 5, Step: 1}}, "5"},
		{"range", []*schedulev1.Range{{Start: 1, End: 5, Step: 1}}, "1-5"},
		{"range with a step", []*schedulev1.Range{{Start: 0, End: 59, Step: 15}}, "0-59/15"},
		{"step without an end matches only the start", []*schedulev1.Range{{Start: 5, Step: 2}}, "5"},
		{"step with an end before the start", []*schedulev1.Range{{Start: 5, End: 3, Step: 2}}, "5"},
		{"several ranges", []*schedulev1.Range{{Start: 1}, {Start: 3, End: 9, Step: 3}}, "1,3-9/3"},
	}
	for _, c := range cases {
		if got := formatRanges(c.ranges); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
