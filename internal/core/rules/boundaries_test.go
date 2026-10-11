package rules

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/BarneyLaw/stele-sync/internal/core/classify"
	"github.com/BarneyLaw/stele-sync/internal/core/vpath"
)

func TestConfigBoundaries(t *testing.T) {
	for _, mut := range []func(*Config){func(c *Config) { c.CanvasRoot = vpath.Path{} }, func(c *Config) { c.MaxAttempts = 10001 }, func(c *Config) { c.Quiet = -1 }, func(c *Config) { c.Window.TextMaxAge = -1 }, func(c *Config) { c.Window.TextMaxOps = -1 }, func(c *Config) { c.Compound = []string{""} }, func(c *Config) { c.Compound = []string{".x", ".X"} }, func(c *Config) { c.Classify.MaxText = 0 }, func(c *Config) { c.Guard.Buckets = 0 }} {
		c := DefaultConfig()
		mut(&c)
		if !errors.Is(c.Validate(), ErrInvalidConfig) {
			t.Fatal(c)
		}
	}
	for _, mut := range []func(*GuardConfig){func(c *GuardConfig) { c.Window = 0 }, func(c *GuardConfig) { c.Window++ }, func(c *GuardConfig) { c.Buckets = 257 }, func(c *GuardConfig) { c.MaxCount = 0 }, func(c *GuardConfig) { c.MinForRatio = 26 }, func(c *GuardConfig) { c.Ratio = math.NaN() }, func(c *GuardConfig) { c.Ratio = math.Inf(1) }, func(c *GuardConfig) { c.Ratio = 0 }, func(c *GuardConfig) { c.Ratio = 2 }} {
		c := DefaultConfig().Guard
		mut(&c)
		if _, e := NewDeleteGuard(c); !errors.Is(e, ErrInvalidConfig) {
			t.Fatal(c)
		}
	}
}
func TestGuardValueAndTime(t *testing.T) {
	g := mustGuard(t, DefaultConfig().Guard)
	at := time.Unix(1000, 1)
	full, d := g.Check(at, Device, 5, 6, false)
	if d != Apply {
		t.Fatal(d)
	}
	for _, tc := range []struct {
		now         time.Time
		batch, live int
	}{{at, -1, 6}, {at, 1, -1}, {at.Add(-time.Hour), 1, 6}} {
		next, d := full.Check(tc.now, Device, tc.batch, tc.live, false)
		if d != NeedsConfirm || !reflect.DeepEqual(full, next) {
			t.Fatal(tc, d)
		}
	}
	if _, d = (DeleteGuard{}).Check(at, Device, 1, 10, false); d != NeedsConfirm {
		t.Fatal(d)
	}
	if _, d = g.Check(at, Actor(255), 1, 10, false); d != NeedsConfirm {
		t.Fatal(d)
	}
	same, d := g.Check(at, Device, 0, 10, false)
	if d != Apply || !reflect.DeepEqual(same, g) {
		t.Fatal("zero batch")
	}
	// Fill each boundary slot, then expire and reuse it without resetting peers.
	cfg := DefaultConfig().Guard
	cfg.Buckets = 2
	cfg.MaxCount = 100
	cfg.MinForRatio = 100
	g = mustGuard(t, cfg)
	for i := 0; i < 5; i++ {
		var decision Decision
		g, decision = g.Check(time.Unix(int64(i*150), 1), Device, 1, 1000, false)
		if decision != Apply {
			t.Fatal(decision)
		}
	}
}
func TestNamesBoundaries(t *testing.T) {
	cfg := DefaultConfig()
	free := func(vpath.Path) (bool, error) { return false, nil }
	for _, name := range []string{"x (abc).md", "x ().md", "x (999999999999999999999999999).md", "x.thisisaverylongextension", "...x.md"} {
		if _, e := FreeName(path(t, name), Marker{}, cfg, free); e != nil {
			t.Fatal(e)
		}
	}
	for _, m := range []Marker{{Kind: MarkerKind(255)}} {
		if _, e := FreeName(path(t, "x"), m, cfg, free); !errors.Is(e, ErrInvalidConfig) {
			t.Fatal(e)
		}
	}
	if _, e := FreeName(vpath.Path{}, Marker{}, cfg, free); !errors.Is(e, ErrInvalidConfig) {
		t.Fatal(e)
	}
	if _, e := FreeName(path(t, "x"), Marker{}, cfg, nil); !errors.Is(e, ErrInvalidConfig) {
		t.Fatal(e)
	}
	n := 0
	p, e := FreeName(path(t, "x.md"), Marker{Kind: Conflict, Device: strings.Repeat("é", 60), At: time.Unix(0, 0).UTC()}, cfg, func(vpath.Path) (bool, error) { n++; return n == 1, nil })
	if e != nil || !strings.Contains(p.String(), " 2)") {
		t.Fatal(p, e)
	}
	pdf := path(t, strings.Repeat("x", 196)+".pdf")
	if _, e := SidecarOf(pdf); !errors.Is(e, ErrNameTooLong) {
		t.Fatal(e)
	}
	q, e := FreeName(pdf, Marker{}, cfg, GroupTaken(free))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = SidecarOf(q); e != nil {
		t.Fatal(e)
	}
	sentinel := errors.New("lookup")
	if _, e := GroupTaken(func(vpath.Path) (bool, error) { return false, sentinel })(path(t, "x.pdf")); !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	if _, e := GroupTaken(free)(path(t, "x.md")); !errors.Is(e, ErrNotPDF) {
		t.Fatal(e)
	}
}
func TestConfiguredRootAndFailClosed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CanvasRoot = path(t, "Courses")
	f := file(t, "courses/a.pdf", classify.Opaque, Canvas)
	if DecidePut(Bridge, f, 10, cfg) != Apply || DecideDelete(Device, f, 10, cfg) != ReadOnlyPath {
		t.Fatal("root ignored")
	}
	f.Owner = User
	if DecideEdit(Actor(255), f, cfg) != ReadOnlyPath || DecideReplace(Bridge, f, 10, cfg) != ReadOnlyPath {
		t.Fatal("authority")
	}
	if Reconcile(classify.Class(0), Gap{}, cfg.Window) != RebaseRequired {
		t.Fatal("unknown class")
	}
	cfg.Classify.TextExts = append(cfg.Classify.TextExts, "code")
	f = file(t, "a.code", classify.Text, User)
	if DecideRename(Device, RenameRequest{Primary: f, To: path(t, "b.txt")}, cfg) != Apply {
		t.Fatal("rename config")
	}
}
