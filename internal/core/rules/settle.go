package rules

import (
	"time"

	"github.com/BarneyLaw/stele-sync/internal/core/classify"
)

// Settlement tells the client whether to wait, submit, or archive class drift.
type Settlement uint8

const (
	// Wait allows transient partial writes to settle.
	Wait Settlement = iota
	// Submit retains the existing class or creates a new file.
	Submit
	// SettleClassChange requests a conflict copy and restoration of the server head.
	// Its prefix avoids colliding with Decision.ClassChange in Go's package scope.
	SettleClassChange
)

// Observation is produced from complete content and an injected quiet duration.
type Observation struct {
	Existing, Confirmed classify.Class
	Demotion            classify.Demotion
	Quiet               time.Duration
}

// Settle never upgrades an existing opaque file and delays content demotions.
func Settle(o Observation, cfg Config) (Settlement, classify.Class) {
	if o.Existing == classify.Opaque {
		return Submit, classify.Opaque
	}
	c := o.Confirmed
	if o.Demotion != classify.Kept || o.Existing != 0 && o.Confirmed == classify.Opaque {
		c = classify.Opaque
		if o.Quiet < cfg.Quiet {
			return Wait, c
		}
	}
	if o.Existing != 0 && o.Existing != c {
		return SettleClassChange, c
	}
	return Submit, c
}
