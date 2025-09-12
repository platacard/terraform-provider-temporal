package provider

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	commonv1 "go.temporal.io/api/common/v1"
	schedulev1 "go.temporal.io/api/schedule/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

func formatDurationCanonical(d *durationpb.Duration) string {
	if d == nil {
		return ""
	}

	totalSeconds := d.GetSeconds()

	// For common intervals, use human-readable format
	switch totalSeconds {
	case 86400: // 24 hours
		return "24h"
	case 3600: // 1 hour
		return "1h"
	case 1800: // 30 minutes
		return "30m"
	case 60: // 1 minute
		return "1m"
	default:
		// For other values, check if it's a clean division
		if totalSeconds%(24*3600) == 0 {
			// Clean days
			return fmt.Sprintf("%dd", totalSeconds/(24*3600))
		} else if totalSeconds%3600 == 0 {
			// Clean hours
			return fmt.Sprintf("%dh", totalSeconds/3600)
		} else if totalSeconds%60 == 0 {
			// Clean minutes
			return fmt.Sprintf("%dm", totalSeconds/60)
		} else {
			// Seconds (Temporal's storage format)
			return fmt.Sprintf("%ds", totalSeconds)
		}
	}
}

// createPayload converts any value to a Temporal Payload.
func createPayload(value string) (*commonv1.Payload, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	return &commonv1.Payload{
		Data: data,
	}, nil
}

// formatRanges converts a slice of Range objects into a comma-separated string representation.
// Each range is formatted as "start[-end][/step]" following cron-like syntax.
func formatRanges(ranges []*schedulev1.Range) string {
	if len(ranges) == 0 {
		return ""
	}

	var builder strings.Builder

	for i, r := range ranges {
		if i > 0 {
			builder.WriteByte(',')
		}
		formatSingleRange(&builder, r)
	}

	return builder.String()
}

// formatSingleRange writes a single range representation to the builder.
func formatSingleRange(builder *strings.Builder, r *schedulev1.Range) {
	if r == nil {
		return
	}

	// Write start value
	builder.WriteString(strconv.Itoa(int(r.Start)))

	// Add end if it's a range (not a single value)
	if r.End > r.Start {
		builder.WriteByte('-')
		builder.WriteString(strconv.Itoa(int(r.End)))
	}

	// Add step if it's not the default
	if r.Step > 1 {
		builder.WriteByte('/')
		builder.WriteString(strconv.Itoa(int(r.Step)))
	}
}
