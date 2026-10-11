package rules

import (
	"reflect"
	"testing"
	"time"

	"github.com/BarneyLaw/stele-sync/internal/core/classify"
	"pgregory.net/rapid"
)

func TestDeleteGuardTable(t *testing.T) {
	cfg := DefaultConfig().Guard
	now := time.Unix(1000, 0)
	for _, tc := range []struct{ live, limit int }{{6, 5}, {80, 8}, {100, 10}, {250, 25}, {20000, 25}} {
		g, e := NewDeleteGuard(cfg)
		if e != nil {
			t.Fatal(e)
		}
		next, d := g.Check(now, Device, tc.limit, tc.live, false)
		if d != Apply {
			t.Fatal(tc, d)
		}
		rejected, d := next.Check(now, Device, 1, tc.live, false)
		if d != NeedsConfirm || !reflect.DeepEqual(rejected, next) {
			t.Fatal("reject changed state")
		}
		if !reflect.DeepEqual(g, mustGuard(t, cfg)) {
			t.Fatal("input mutated")
		}
		for _, actor := range []Actor{Bridge, Admin} {
			r, d := next.Check(now, actor, 100, tc.live, false)
			if d != Apply || !reflect.DeepEqual(r, next) {
				t.Fatal("system")
			}
		}
		r, d := next.Check(now, Device, 100, tc.live, true)
		if d != Apply || !reflect.DeepEqual(r, next) {
			t.Fatal("confirm")
		}
		// An event expires no earlier than Window, at most one bucket late.
		if _, d = next.Check(now.Add(cfg.Window-time.Nanosecond), Device, 1, tc.live, false); d != NeedsConfirm {
			t.Fatal("early expiry")
		}
		if _, d = next.Check(now.Add(cfg.Window+cfg.Window/time.Duration(cfg.Buckets)), Device, 1, tc.live, false); d != Apply {
			t.Fatal("late expiry")
		}
	}
}
func mustGuard(t testing.TB, c GuardConfig) DeleteGuard {
	t.Helper()
	g, e := NewDeleteGuard(c)
	if e != nil {
		t.Fatal(e)
	}
	return g
}
func TestPropDeleteGuard(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cfg := DefaultConfig().Guard
		g, err := NewDeleteGuard(cfg)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Unix(0, 0)
		type event struct {
			at time.Time
			n  int
		}
		var events []event
		live := rapid.IntRange(6, 300).Draw(t, "live")
		limit := min(25, max(5, live/10))
		for i := 0; i < 20; i++ {
			now = now.Add(time.Duration(rapid.IntRange(0, 80).Draw(t, "seconds")) * time.Second)
			batch := rapid.IntRange(0, 10).Draw(t, "batch")
			before := g
			next, d := g.Check(now, Device, batch, live, false)
			if d == NeedsConfirm {
				if before != next {
					t.Fatal("reject mutated")
				}
				continue
			}
			events = append(events, event{now, batch})
			n := 0
			for _, e := range events {
				if now.Sub(e.at) < cfg.Window {
					n += e.n
				}
			}
			if n > limit {
				t.Fatal("unsafe sliding window", n, limit)
			}
			g = next
		}
	})
}
func TestSettle(t *testing.T) {
	cfg := DefaultConfig()
	for _, tc := range []struct {
		o Observation
		s Settlement
		c classify.Class
	}{{Observation{Confirmed: classify.Text}, Submit, classify.Text}, {Observation{Existing: classify.Text, Confirmed: classify.Text}, Submit, classify.Text}, {Observation{Confirmed: classify.Opaque, Demotion: classify.InvalidUTF8}, Wait, classify.Opaque}, {Observation{Confirmed: classify.Opaque, Demotion: classify.InvalidUTF8, Quiet: 10 * time.Second}, Submit, classify.Opaque}, {Observation{Existing: classify.Text, Confirmed: classify.Opaque, Demotion: classify.InvalidUTF8, Quiet: 10 * time.Second}, SettleClassChange, classify.Opaque}, {Observation{Existing: classify.Opaque, Confirmed: classify.Sidecar}, Submit, classify.Opaque}, {Observation{Existing: classify.Sidecar, Confirmed: classify.Opaque, Demotion: classify.NotFreedraw, Quiet: 10*time.Second - 1}, Wait, classify.Opaque}} {
		s, c := Settle(tc.o, cfg)
		if s != tc.s || c != tc.c {
			t.Fatal(tc, s, c)
		}
	}
}

func TestMetadataDemotionWaits(t *testing.T) {
	cfg := DefaultConfig()
	// Expect on an oversized .md is already C, so Confirm returns Kept. The
	// existing class must still delay that demotion until the quiet boundary.
	o := Observation{Existing: classify.Text, Confirmed: classify.Opaque, Demotion: classify.Kept}
	if s, _ := Settle(o, cfg); s != Wait {
		t.Fatal("metadata demotion bypassed quiet period")
	}
	o.Quiet = cfg.Quiet
	if s, c := Settle(o, cfg); s != SettleClassChange || c != classify.Opaque {
		t.Fatal(s, c)
	}
}
