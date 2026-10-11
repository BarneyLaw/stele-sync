package rules

import (
	"math"
	"time"
)

// GuardConfig bounds the pure deletion window. Buckets is in [1,256].
type GuardConfig struct {
	Window                time.Duration
	MaxCount, MinForRatio int
	Ratio                 float64
	Buckets               int
}

// Validate checks the bounded ring, exact slot duration and finite ratio.
func (c GuardConfig) Validate() error {
	if c.Window <= 0 || c.Buckets < 1 || c.Buckets > 256 || c.Window%time.Duration(c.Buckets) != 0 || c.MaxCount < 1 || c.MinForRatio < 1 || c.MinForRatio > c.MaxCount || math.IsNaN(c.Ratio) || math.IsInf(c.Ratio, 0) || c.Ratio <= 0 || c.Ratio > 1 {
		return ErrInvalidConfig
	}
	return nil
}

type bucket struct {
	Until time.Time
	Count int
}

// DeleteGuard owns a fixed ring by value, including one conservative boundary slot.
// Copying a guard cannot alias its counts. The zero value fails closed.
type DeleteGuard struct {
	cfg   GuardConfig
	slots [257]bucket
	last  time.Time
}

// NewDeleteGuard validates the config and returns an empty immutable value.
func NewDeleteGuard(cfg GuardConfig) (DeleteGuard, error) {
	if e := cfg.Validate(); e != nil {
		return DeleteGuard{}, e
	}
	return DeleteGuard{cfg: cfg}, nil
}

// Check records accepted unconfirmed device deletes only. Rejections, confirmed
// batches and system actors return the input unchanged. Time cannot move backwards.
func (g DeleteGuard) Check(now time.Time, a Actor, batch, live int, confirmed bool) (DeleteGuard, Decision) {
	if a == Bridge || a == Admin || confirmed {
		return g, Apply
	}
	if a != Device || g.cfg.Buckets == 0 || batch < 0 || live < 0 {
		return g, NeedsConfirm
	}
	if now.Before(g.last) {
		now = g.last
	}
	limit := g.cfg.MaxCount
	scaled := math.Floor(g.cfg.Ratio * float64(live))
	if scaled < float64(limit) {
		limit = max(g.cfg.MinForRatio, int(scaled))
	}
	if batch > limit {
		return g, NeedsConfirm
	}
	remaining := limit - batch
	for _, b := range g.slots[:g.cfg.Buckets+1] {
		if now.Before(b.Until) {
			if b.Count > remaining {
				return g, NeedsConfirm
			}
			remaining -= b.Count
		}
	}
	if batch == 0 {
		return g, Apply
	}
	slot := g.cfg.Window / time.Duration(g.cfg.Buckets)
	until := now.Truncate(slot).Add(slot).Add(g.cfg.Window)
	// At most Buckets+1 distinct expiration times coexist. Avoid UnixNano so
	// dates outside its 1678–2262 range and zero-time tests remain well-defined.
	index := -1
	for i, b := range g.slots[:g.cfg.Buckets+1] {
		if b.Until.Equal(until) {
			index = i
			break
		}
		if !now.Before(b.Until) {
			index = i
		}
	}
	if index < 0 {
		return g, NeedsConfirm
	} // Defensive fail-closed for malformed state.
	next := g
	if !next.slots[index].Until.Equal(until) {
		next.slots[index] = bucket{Until: until}
	}
	next.slots[index].Count += batch
	next.last = now
	return next, Apply
}
