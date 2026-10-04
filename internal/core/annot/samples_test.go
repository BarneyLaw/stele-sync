package annot_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/BarneyLaw/stele-sync/internal/core/annot"
)

func TestSamplesRoundTrip(t *testing.T) {
	names, err := filepath.Glob("../../../schema/annot/samples/*.annot.json")
	if err != nil || len(names) == 0 {
		t.Fatalf("missing captured samples: %v", err)
	}
	outDir := os.Getenv("ANNOT_OUTPUT_DIR")
	for _, name := range names {
		t.Run(filepath.Base(name), func(t *testing.T) {
			raw, err := os.ReadFile(name) // #nosec G304 G703 -- paths come from a fixed repository fixture glob.
			if err != nil {
				t.Fatal(err)
			}
			sc, err := annot.Parse(raw, annot.DefaultLimits())
			if err != nil {
				t.Fatal(err)
			}
			canonical := materialize(t, sc)
			again, err := annot.Parse(canonical, annot.DefaultLimits())
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(canonical, materialize(t, again)) {
				t.Fatal("sample is not idempotent")
			}
			state, err := annot.EncodeState(sc)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := annot.DecodeState(state, annot.DefaultLimits())
			if err != nil || annot.CanonicalHash(restored) != annot.CanonicalHash(sc) {
				t.Fatalf("sample state round trip: %v", err)
			}
			if outDir != "" {
				if err := os.MkdirAll(outDir, 0700); err != nil { // #nosec G703 -- explicit opt-in test output directory.
					t.Fatal(err)
				}
				// Explicit opt-in output for isolated Obsidian compatibility verification.
				if err := os.WriteFile(filepath.Join(outDir, filepath.Base(name)), canonical, 0600); err != nil {
					t.Fatal(err)
				} // #nosec G703 -- test-owned output directory.
			}
		})
	}
}
