//go:build oracle_scale

package textop_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func oracleInteger(t *testing.T, name, fallback string, lower, upper uint64) uint64 {
	t.Helper()
	s := os.Getenv(name)
	if s == "" {
		s = fallback
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil || strconv.FormatUint(n, 10) != s || n < lower || n > upper {
		t.Fatalf("%s must be decimal %d..%d", name, lower, upper)
	}
	return n
}

func TestOracleScale(t *testing.T) {
	count := oracleInteger(t, "ORACLE_COUNT", "10000000", 618, 10000000)
	seed := oracleInteger(t, "ORACLE_SEED", "1", 0, 4294967295)
	hash, err := generatorHash()
	if err != nil {
		t.Fatal(err)
	}
	h := oracleHeader{SchemaVersion: 1, GeneratorHash: hash, OTVersion: "0.0.15", TieBreak: "a-first", Kind: "all", Profile: "scale", Seed: seed, Count: int(count), LargePerKind: 100, ComplexPerKind: 100} // #nosec G115 -- oracleInteger bounds count to 10,000,000, including on 32-bit hosts.
	deadline := 110 * time.Minute
	if s := os.Getenv("ORACLE_TIMEOUT"); s != "" {
		deadline, err = time.ParseDuration(s)
		if err != nil || deadline <= 0 {
			t.Fatal("invalid ORACLE_TIMEOUT")
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), deadline)
	defer cancel()
	node := os.Getenv("ORACLE_NODE")
	if node == "" {
		node = "node"
	}
	args := []string{filepath.Join(repoRoot(), "tools/oracle/generate.mjs"), "--stream", "--profile", "scale", "--seed", strconv.FormatUint(seed, 10), "--count", strconv.FormatUint(count, 10)}
	t.Logf("oracle seed=%d count=%d hash=%s deadline=%s", seed, count, hash, deadline)
	stats, failure := runOracle(ctx, node, args, os.Environ(), h)
	summary, err := json.Marshal(struct {
		Header   oracleHeader   `json:"header"`
		Coverage oracleCoverage `json:"coverage"`
		Passed   bool           `json:"passed"`
	}{h, stats, failure == nil})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("measured coverage: %s", summary)
	if path := os.Getenv("ORACLE_SUMMARY"); path != "" {
		if err := os.WriteFile(path, append(summary, '\n'), 0600); err != nil { // #nosec G703 -- explicit operator-selected artifact path; fixture data never supplies paths.
			t.Error(err)
		}
	}
	if failure != nil {
		if len(failure.Header) == 0 {
			failure.Header, err = json.Marshal(h)
			if err != nil {
				t.Fatal(err)
			}
		}
		path := os.Getenv("ORACLE_FAILURES")
		if path == "" {
			path = filepath.Join(repoRoot(), ".cache/oracle-failures.ndjson")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil { // #nosec G703 -- explicit operator-selected artifact path.
			t.Fatal(err)
		}
		raw, err := json.Marshal(failure)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(raw, '\n'), 0600); err != nil { // #nosec G703 -- explicit operator-selected artifact path.
			t.Fatal(err)
		}
		t.Fatalf("oracle failed; complete replay record: %s\n%.4000s\nstderr tail: %.16000s", path, failure.Diagnostic, failure.Stderr)
	}
}

func TestOracleReplay(t *testing.T) {
	path := os.Getenv("ORACLE_REPLAY")
	if path == "" {
		t.Fatal("ORACLE_REPLAY must name a failure NDJSON artifact")
	}
	f, err := os.Open(path) // #nosec G304 G703 -- replay intentionally reads the file named by the operator.
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 65536), 2*maxRecordBytes)
	n := 0
	for scan.Scan() {
		var failure oracleFailure
		if err := strictJSON(scan.Bytes(), &failure); err != nil {
			t.Fatal(err)
		}
		if len(failure.Case) == 0 {
			t.Fatal("infrastructure failure has no case to replay; inspect diagnostic and stderr")
		}
		var recorded oracleHeader
		if err := strictJSON(failure.Header, &recorded); err != nil {
			t.Fatal(err)
		}
		// Saved cases survive generator revisions, but must satisfy the same schema,
		// pinned oracle version and tie policy. Log both hashes for provenance.
		if _, err := readHeader(failure.Header, recorded.GeneratorHash); err != nil {
			t.Fatal(err)
		}
		c, err := readCase(failure.Case)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(fmt.Sprintf("%s-%d", c.ID, n), func(t *testing.T) {
			t.Logf("original seed=%d hash=%s", recorded.Seed, recorded.GeneratorHash)
			if err := compareCase(c); err != nil {
				t.Fatal(err)
			}
		})
		n++
	}
	if err := scan.Err(); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("empty replay artifact")
	}
}
