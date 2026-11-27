package provider

import (
	"testing"
	"time"

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
