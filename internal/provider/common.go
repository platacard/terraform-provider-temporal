package provider

import (
	"go.temporal.io/api/enums/v1"
)

var (
	ArchivalState = map[string]enums.ArchivalState{
		"Unspecified": enums.ARCHIVAL_STATE_UNSPECIFIED,
		"Disabled":    enums.ARCHIVAL_STATE_DISABLED,
		"Enabled":     enums.ARCHIVAL_STATE_ENABLED,
	}
)
