package textop_test

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/BarneyLaw/stele-sync/internal/core/textop"
)

func applyWorkload(t fataler, size int, replace bool) (textop.Doc, textop.Op) {
	text := strings.Repeat("a", size)
	d := mustDoc(t, text)
	var b textop.Builder
	if replace { // 64 single-byte substitutions; exact output size is unchanged.
		stride := size / 64
		for range 64 {
			b.Retain(stride - 1).Insert("X").Delete(1)
		}
		b.Retain(size % 64)
	} else {
		b.Retain(size)
	}
	op, err := b.Op()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, err := applyChecked(d, op); err != nil {
		t.Fatalf("apply setup: %v", err)
	}
	return d, op
}

func benchmarkApply(b *testing.B, size int, replace bool) {
	d, op := applyWorkload(b, size, replace)
	b.SetBytes(int64(size))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := textop.Apply(d, op); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkApply(b *testing.B) {
	for _, size := range []int{50000, 5000000} {
		b.Run(fmt.Sprintf("%d-bytes", size), func(b *testing.B) { benchmarkApply(b, size, true) })
	}
}
func BenchmarkApplyLarge(b *testing.B) {
	for _, replace := range []bool{false, true} {
		b.Run(fmt.Sprintf("8388608-bytes/replace=%t", replace), func(b *testing.B) { benchmarkApply(b, 8<<20, replace) })
	}
}

type gapWorkload struct {
	incoming textop.Op
	history  []textop.Op
	head     textop.Doc
	want     string
}

func makeGap(t fataler, unicode bool) gapWorkload {
	base := "base"
	if unicode {
		base = strings.Repeat("a😀中b", 8)
	}
	var incoming textop.Builder
	// The marker is after the base. Appended history wins same-position ties,
	// so the independently expected marker remains after the entire new head.
	incoming.Retain(ulen(base)).Insert("!")
	op, err := incoming.Op()
	if err != nil {
		t.Fatalf("incoming: %v", err)
	}
	w := gapWorkload{incoming: op}
	head := mustDoc(t, base)
	model := []rune(base)
	for step := 0; step < 5000; step++ {
		var b textop.Builder
		if unicode {
			next := append([]rune(nil), model...)
			for i, r := range model {
				if i%2 == 0 {
					replacement := '界'
					if step%2 == 1 {
						replacement = '語'
					}
					b.Insert(string(replacement)).Delete(ulen(string(r)))
					next[i] = replacement
				} else {
					b.Retain(ulen(string(r)))
				}
			}
			model = next
		} else {
			b.Retain(len(model)).Insert("x")
			model = append(model, 'x')
		}
		h, err := b.Op()
		if err != nil {
			t.Fatalf("history %d: %v", step, err)
		}
		if h.BaseLen() != head.Len() {
			t.Fatalf("history is not sequential at %d", step)
		}
		head, err = textop.Apply(head, h)
		if err != nil || head.String() != string(model) {
			t.Fatalf("history model differs at %d: %v", step, err)
		}
		w.history = append(w.history, h)
	}
	w.head = head
	w.want = string(model) + "!"
	rebased, err := rebaseGap(w)
	if err != nil {
		t.Fatalf("rebase: %v", err)
	}
	got, err := applyChecked(head, rebased)
	if err != nil || got.String() != w.want {
		t.Fatalf("gap expected text differs: %v", err)
	}
	return w
}

func rebaseGap(w gapWorkload) (textop.Op, error) {
	op := w.incoming
	for _, h := range w.history {
		var err error
		_, op, err = textop.Transform(h, op)
		if err != nil {
			return textop.Op{}, err
		}
	}
	return op, nil
}

func BenchmarkTransformGap(b *testing.B) {
	for _, unicode := range []bool{false, true} {
		b.Run(fmt.Sprintf("5000-ops/unicode=%t", unicode), func(b *testing.B) {
			w := makeGap(b, unicode)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := rebaseGap(w); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestTextopPerformance(t *testing.T) {
	if os.Getenv("TEXTOP_PERF") != "1" {
		t.Skip("dedicated non-race performance gate: node tools/tasks.mjs textop-perf")
	}
	samples := 1000
	if s := os.Getenv("TEXTOP_PERF_SAMPLES"); s != "" {
		var err error
		samples, err = strconv.Atoi(s)
		if err != nil || samples < 100 {
			t.Fatal("need at least 100 samples")
		}
	}
	t.Logf("CPU=%s GOOS=%s GOARCH=%s Go=%s compiler=%s GOMAXPROCS=%d seed=0 workload=v1 samples=%d flags=go test (no race)", os.Getenv("TEXTOP_CPU"), runtime.GOOS, runtime.GOARCH, runtime.Version(), runtime.Compiler, runtime.GOMAXPROCS(0), samples)
	measure := func(t *testing.T, run func() error) {
		for range 10 {
			if err := run(); err != nil {
				t.Fatal(err)
			}
		}
		times := make([]time.Duration, samples)
		for i := range times {
			start := time.Now()
			err := run()
			times[i] = time.Since(start)
			if err != nil {
				t.Fatal(err)
			}
		}
		slices.Sort(times)
		pct := func(p float64) time.Duration { return times[int(math.Ceil(p*float64(samples)))-1] }
		t.Logf("p50=%s p95=%s p99=%s", pct(.50), pct(.95), pct(.99))
		if pct(.99) >= 50*time.Millisecond {
			t.Errorf("p99 %s exceeds 50 ms", pct(.99))
		}
	}
	for _, unicode := range []bool{false, true} {
		t.Run(fmt.Sprintf("gap/unicode=%t", unicode), func(t *testing.T) {
			w := makeGap(t, unicode)
			measure(t, func() error { _, err := rebaseGap(w); return err })
		})
	}
	for _, replace := range []bool{false, true} {
		t.Run(fmt.Sprintf("apply/replace=%t", replace), func(t *testing.T) {
			d, op := applyWorkload(t, 8<<20, replace)
			measure(t, func() error { _, err := textop.Apply(d, op); return err })
		})
	}
}
