package rules

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/BarneyLaw/stele-sync/internal/core/classify"
)

var updateRules = flag.Bool("update-rules", false, "regenerate guard and settling contracts")

type guardStep struct {
	Now       string `json:"now_ns"`
	Actor     string `json:"actor"`
	Batch     int    `json:"batch"`
	Live      int    `json:"live"`
	Confirmed bool   `json:"confirmed"`
	Decision  string `json:"decision"`
}
type guardFixture struct {
	Name     string      `json:"name"`
	Window   string      `json:"window_ns"`
	MaxCount int         `json:"max_count"`
	Min      int         `json:"min_for_ratio"`
	Ratio    float64     `json:"ratio"`
	Buckets  int         `json:"buckets"`
	Steps    []guardStep `json:"steps"`
}
type settleRow struct {
	Existing   string `json:"existing"`
	Confirmed  string `json:"confirmed"`
	Demotion   string `json:"demotion"`
	Quiet      string `json:"quiet_ns"`
	Settlement string `json:"settlement"`
	Class      string `json:"class"`
}

func TestRulesFixtures(t *testing.T) {
	cfg := DefaultConfig()
	var guards []guardFixture
	for _, live := range []int{6, 80, 100, 250, 20000} {
		g := mustGuard(t, cfg.Guard)
		row := guardFixture{Name: "window", Window: "300000000000", MaxCount: 25, Min: 5, Ratio: 0.1, Buckets: 30}
		for _, step := range []struct {
			now       int64
			actor     Actor
			batch     int
			confirmed bool
		}{{1, Device, 5, false}, {2, Device, 1, false}, {3, Device, 25, true}, {4, Bridge, 100, false}, {5, Admin, 100, false}, {6, Device, 1, false}, {300000000000, Device, 25, false}, {310000000000, Device, 5, false}, {-1, Device, 5, false}, {620000000000, Device, 1, false}} {
			next, d := g.Check(time.Unix(0, step.now), step.actor, step.batch, live, step.confirmed)
			actor := []string{"device", "bridge", "admin"}[step.actor]
			decision := "apply"
			if d == NeedsConfirm {
				decision = "needs_confirm"
			}
			row.Steps = append(row.Steps, guardStep{timeNS(step.now), actor, step.batch, live, step.confirmed, decision})
			g = next
		}
		guards = append(guards, row)
	}
	var settle []settleRow
	for _, existing := range []classify.Class{0, classify.Text, classify.Sidecar, classify.Opaque} {
		for _, confirmed := range []classify.Class{classify.Text, classify.Sidecar, classify.Opaque} {
			for _, d := range []classify.Demotion{classify.Kept, classify.TooLarge, classify.NotFreedraw} {
				for _, quiet := range []int64{0, 9999999999, 10000000000, 10000000001} {
					s, c := Settle(Observation{existing, confirmed, d, time.Duration(quiet)}, cfg)
					name := []string{"wait", "submit", "class_change"}[s]
					e := ""
					if existing != 0 {
						e = string(existing)
					}
					settle = append(settle, settleRow{e, string(confirmed), d.String(), timeNS(quiet), name, string(c)})
				}
			}
		}
	}
	rulesFixture(t, "guard", guards)
	rulesFixture(t, "settle", settle)
}
func timeNS(n int64) string { return strconv.FormatInt(n, 10) }
func rulesFixture(t *testing.T, name string, v any) {
	t.Helper()
	data, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	data = append(data, '\n')
	dir := "../../../schema/rules"
	p := dir + "/" + name + ".json"
	if *updateRules {
		if e := os.MkdirAll(dir, 0750); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(p, data, 0600); e != nil {
			t.Fatal(e)
		}
	}
	got, e := os.ReadFile(p) // #nosec G304 -- fixed repository fixture path.
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("%s fixture drift", name)
	}
}
