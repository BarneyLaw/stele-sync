package textop_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type oracleCoverage struct {
	Count               int            `json:"count"`
	Kinds               map[string]int `json:"kinds"`
	Large               map[string]int `json:"large"`
	Complex             map[string]int `json:"complex"`
	Categories          map[string]int `json:"categories"`
	MaxInputComponents  int            `json:"max_input_components"`
	MaxOutputComponents int            `json:"max_output_components"`
	MaxRecordBytes      int            `json:"max_record_bytes"`
}

func newCoverage() oracleCoverage {
	return oracleCoverage{Kinds: map[string]int{}, Large: map[string]int{}, Complex: map[string]int{}, Categories: map[string]int{}}
}

func (s *oracleCoverage) record(c oracleCase, size int) error {
	input, output := 0, 0
	for _, raw := range []json.RawMessage{c.Op, c.A, c.B} {
		if raw != nil {
			var parts []json.RawMessage
			if err := json.Unmarshal(raw, &parts); err != nil {
				return err
			}
			input = max(input, len(parts))
		}
	}
	if c.Kind == "compose" {
		var parts []json.RawMessage
		if err := json.Unmarshal(c.Want, &parts); err != nil {
			return err
		}
		output = len(parts)
	}
	if c.Kind == "transform" {
		for _, raw := range []json.RawMessage{c.WantAP, c.WantBP} {
			var parts []json.RawMessage
			if err := json.Unmarshal(raw, &parts); err != nil {
				return err
			}
			output = max(output, len(parts))
		}
	}
	s.Count++
	s.Kinds[c.Kind]++
	s.Categories[c.Kind+"/"+c.Category]++
	if len(c.Doc) == 1<<20 {
		s.Large[c.Kind]++
	}
	if input >= 500 {
		s.Complex[c.Kind]++
	}
	if c.Category == "large" && len(c.Doc) != 1<<20 {
		return fmt.Errorf("claimed large case is not exactly 1 MiB")
	}
	if c.Category == "complex" && input < 500 {
		return fmt.Errorf("claimed complex case has fewer than 500 components")
	}
	s.MaxInputComponents = max(s.MaxInputComponents, input)
	s.MaxOutputComponents = max(s.MaxOutputComponents, output)
	s.MaxRecordBytes = max(s.MaxRecordBytes, size)
	return nil
}

type oracleFailure struct {
	Header     json.RawMessage `json:"header,omitempty"`
	Case       json.RawMessage `json:"case,omitempty"`
	Diagnostic string          `json:"diagnostic"`
	Stderr     string          `json:"stderr,omitempty"`
}

func (f *oracleFailure) Error() string { return f.Diagnostic }

func terminatedLines(data []byte, eof bool) (int, []byte, error) {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return i + 1, data[:i], nil
	}
	if eof && len(data) > 0 {
		return 0, nil, io.ErrUnexpectedEOF
	}
	return 0, nil, nil
}

func consumeOracle(r io.Reader, expected oracleHeader, ceiling int) (oracleCoverage, *oracleFailure) {
	stats := newCoverage()
	scan := bufio.NewScanner(r)
	scan.Buffer(make([]byte, 65536), ceiling)
	scan.Split(terminatedLines)
	var header json.RawMessage
	fail := func(raw []byte, err error) (oracleCoverage, *oracleFailure) {
		var c json.RawMessage
		if json.Valid(raw) {
			c = bytes.Clone(raw)
		}
		return stats, &oracleFailure{Header: header, Case: c, Diagnostic: err.Error()}
	}
	if !scan.Scan() {
		return fail(nil, fmt.Errorf("missing header: %s", fmt.Sprint(scan.Err())))
	}
	var wrapper struct {
		Header json.RawMessage `json:"header"`
	}
	if err := exactFields(scan.Bytes(), "header", ""); err != nil {
		return fail(nil, fmt.Errorf("header: %w", err))
	}
	if err := strictJSON(scan.Bytes(), &wrapper); err != nil {
		return fail(nil, err)
	}
	header = bytes.Clone(wrapper.Header)
	h, err := readHeader(header, expected.GeneratorHash)
	if err != nil {
		return fail(nil, err)
	}
	if h != expected {
		return fail(nil, fmt.Errorf("header differs from requested run: got %+v want %+v", h, expected))
	}
	kinds := []string{h.Kind}
	if h.Kind == "all" {
		kinds = []string{"apply", "compose", "transform"}
	}
	categories := []string{"identity", "unicode", "tie-cancel", "overlap", "delete", "replace"}
	for scan.Scan() {
		raw := scan.Bytes()
		c, err := readCase(raw)
		if err != nil {
			return fail(raw, fmt.Errorf("record %d: %w", stats.Count, err))
		}
		i := stats.Count
		ordinal := i / len(kinds)
		kind := kinds[i%len(kinds)]
		if i >= h.Count || c.Index != i || c.Kind != kind || c.ID != fmt.Sprintf("%s-%d", kind, ordinal) {
			return fail(raw, fmt.Errorf("noncontiguous or unexpected record %d", i))
		}
		category := "random"
		switch {
		case ordinal >= 0 && ordinal < len(categories):
			category = categories[ordinal] // #nosec G602 -- guarded by 0 <= ordinal < len(categories) above.
		case h.Profile == "scale" && ordinal < 106:
			category = "large"
		case h.Profile == "scale" && ordinal < 206:
			category = "complex"
		}
		if c.Category != category {
			return fail(raw, fmt.Errorf("mandatory category at %d: got %s want %s", i, c.Category, category))
		}
		if err := compareCase(c); err != nil {
			return fail(raw, fmt.Errorf("%s: %w", c.Kind, err))
		}
		if err := stats.record(c, len(raw)); err != nil {
			return fail(raw, err)
		}
	}
	if scan.Err() != nil {
		return fail(nil, fmt.Errorf("stream read: %w", scan.Err()))
	}
	if stats.Count != h.Count {
		return fail(nil, fmt.Errorf("count: got %d want %d", stats.Count, h.Count))
	}
	for i, kind := range kinds {
		want := (h.Count + len(kinds) - 1 - i) / len(kinds)
		if stats.Kinds[kind] != want || stats.Large[kind] < h.LargePerKind || stats.Complex[kind] < h.ComplexPerKind {
			return fail(nil, fmt.Errorf("kind/size quotas not met for %s", kind))
		}
		for _, category := range categories {
			if stats.Categories[kind+"/"+category] == 0 {
				return fail(nil, fmt.Errorf("missing %s/%s", kind, category))
			}
		}
	}
	return stats, nil
}

// os/exec drains this writer on its own goroutine. Read only after Wait joins it.
type tailBuffer struct{ data []byte }

func (b *tailBuffer) Write(p []byte) (int, error) {
	n := len(p)
	const capacity = 16 << 10
	if len(p) >= capacity {
		b.data = append(b.data[:0], p[len(p)-capacity:]...)
	} else {
		excess := len(b.data) + len(p) - capacity
		if excess > 0 {
			b.data = b.data[excess:]
		}
		b.data = append(b.data, p...)
	}
	return n, nil
}

func runOracle(ctx context.Context, command string, args, env []string, expected oracleHeader) (oracleCoverage, *oracleFailure) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, command, args...) // #nosec G204 G702 -- explicit test binary/operator-selected Node executable, never a shell or fixture command.
	cmd.Env = env
	cmd.Dir = repoRoot()
	cmd.WaitDelay = 2 * time.Second
	var stderr tailBuffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return newCoverage(), &oracleFailure{Diagnostic: "stdout pipe: " + err.Error()}
	}
	if err := cmd.Start(); err != nil {
		_ = out.Close()
		return newCoverage(), &oracleFailure{Diagnostic: "start oracle: " + err.Error()}
	}
	stats, failure := consumeOracle(out, expected, maxRecordBytes)
	if failure != nil {
		cancel()
		_ = out.Close()
	}
	waitErr := cmd.Wait() // always reap, including comparator failure and cancellation
	if failure == nil && waitErr != nil {
		failure = &oracleFailure{Diagnostic: "oracle process: " + waitErr.Error()}
	}
	if failure != nil {
		failure.Stderr = string(stderr.data)
		if ctx.Err() != nil && waitErr != nil {
			failure.Diagnostic += "; process: " + waitErr.Error()
		}
	}
	return stats, failure
}

func syntheticStream(t *testing.T) (oracleHeader, []byte) {
	t.Helper()
	h := oracleHeader{SchemaVersion: 1, GeneratorHash: strings.Repeat("a", 64), OTVersion: "0.0.15", TieBreak: "a-first", Kind: "apply", Profile: "fixtures", Seed: 1, Count: 6}
	raw, err := json.Marshal(struct {
		Header oracleHeader `json:"header"`
	}{h})
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	for i, category := range []string{"identity", "unicode", "tie-cancel", "overlap", "delete", "replace"} {
		c := oracleCase{ID: fmt.Sprintf("apply-%d", i), Kind: "apply", Index: i, Category: category, Doc: "", Op: json.RawMessage(`[]`), Want: json.RawMessage(`""`)}
		line, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		raw = append(raw, line...)
		raw = append(raw, '\n')
	}
	return h, raw
}

func TestOracleStreamFailures(t *testing.T) {
	h, valid := syntheticStream(t)
	if stats, f := consumeOracle(bytes.NewReader(valid), h, maxRecordBytes); f != nil || stats.Count != 6 {
		t.Fatalf("valid stream: %+v %v", stats, f)
	}
	lines := bytes.Split(valid, []byte{'\n'})
	for name, raw := range map[string][]byte{
		"missing-header": nil, "bad-header": []byte("{}\n"), "truncated": valid[:len(valid)-1],
		"skipped-index":          bytes.Replace(valid, []byte(`"i":1`), []byte(`"i":2`), 1),
		"malformed":              append(bytes.Clone(lines[0]), []byte("\n{bad}\n")...),
		"nonzero-error-contract": bytes.Replace(valid, []byte(`"want":""`), []byte(`"error":"mystery"`), 1),
		"unsupported-version":    bytes.Replace(valid, []byte(`"schema_version":1`), []byte(`"schema_version":2`), 1),
		"missing-field":          bytes.Replace(valid, []byte(`"doc":"",`), nil, 1),
		"wrong-result":           bytes.Replace(valid, []byte(`"want":""`), []byte(`"want":"x"`), 1),
		"extra-record":           append(bytes.Clone(valid), append(bytes.Clone(lines[1]), '\n')...),
	} {
		t.Run(name, func(t *testing.T) {
			if _, f := consumeOracle(bytes.NewReader(raw), h, maxRecordBytes); f == nil {
				t.Fatal("accepted corrupt stream")
			}
		})
	}
	shortStream := append(bytes.Join(lines[:len(lines)-2], []byte{'\n'}), '\n')
	if stats, f := consumeOracle(bytes.NewReader(shortStream), h, maxRecordBytes); f == nil || stats.Count != 5 || !strings.Contains(f.Diagnostic, "count: got 5 want 6") {
		t.Fatalf("wrong-count path: count=%d failure=%v", stats.Count, f)
	}
	// This record is valid in every other respect. Without the reader ceiling,
	// it must pass; invalid JSON would not prove that size enforcement works.
	longDoc := strings.Repeat("x", 40000)
	longWant, err := json.Marshal(longDoc)
	if err != nil {
		t.Fatal(err)
	}
	big := oracleCase{ID: "apply-0", Kind: "apply", Index: 0, Category: "identity", Doc: longDoc, Op: json.RawMessage(`[40000]`), Want: longWant}
	bigLine, err := json.Marshal(big)
	if err != nil {
		t.Fatal(err)
	}
	bigLines := append([][]byte(nil), lines...)
	bigLines[1] = bigLine
	bigStream := bytes.Join(bigLines, []byte{'\n'})
	if _, f := consumeOracle(bytes.NewReader(bigStream), h, maxRecordBytes); f != nil {
		t.Fatalf("otherwise valid large record: %v", f)
	}
	if _, f := consumeOracle(bytes.NewReader(bigStream), h, 65536); f == nil || !strings.Contains(f.Diagnostic, "token too long") {
		t.Fatalf("record ceiling not enforced: %v", f)
	}
	if _, f := consumeOracle(io.MultiReader(bytes.NewReader(valid), brokenReader{}), h, maxRecordBytes); f == nil {
		t.Fatal("lost reader error")
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("injected reader failure") }

func TestOracleProcess(t *testing.T) {
	if mode := os.Getenv("TEXTOP_ORACLE_HELPER"); mode != "" {
		_, raw := syntheticStream(t)
		switch mode {
		case "timeout":
			time.Sleep(30 * time.Second)
		case "nonzero":
			if _, err := os.Stderr.WriteString(strings.Repeat("e", 128<<10) + "tail-end"); err != nil {
				os.Exit(4)
			}
			if _, err := os.Stdout.Write(raw); err != nil {
				os.Exit(4)
			}
			os.Exit(7)
		case "stderr":
			if _, err := os.Stderr.WriteString(strings.Repeat("e", 128<<10)); err != nil {
				os.Exit(4)
			}
			if _, err := os.Stdout.Write(raw); err != nil {
				os.Exit(4)
			}
			os.Exit(0)
		case "cancel":
			if _, err := os.Stdout.WriteString("{}\n"); err != nil {
				os.Exit(4)
			}
			time.Sleep(30 * time.Second)
		}
		os.Exit(0)
	}
	h, _ := syntheticStream(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"nonzero", "timeout", "stderr", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			deadline := 5 * time.Second
			if mode == "timeout" {
				deadline = 200 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(t.Context(), deadline)
			defer cancel()
			stats, failure := runOracle(ctx, exe, []string{"-test.run=^TestOracleProcess$"}, append(os.Environ(), "TEXTOP_ORACLE_HELPER="+mode), h)
			if mode == "stderr" {
				if stats.Count != 6 {
					t.Fatalf("count=%d", stats.Count)
				}
				if failure != nil {
					t.Fatal(failure)
				}
			} else if failure == nil {
				t.Fatal("lost child failure")
			}
			if mode == "nonzero" {
				if stats.Count != 6 || !strings.Contains(failure.Diagnostic, "exit status 7") {
					t.Fatalf("wrong process failure: count=%d %v", stats.Count, failure)
				}
				if len(failure.Stderr) > 16<<10 || !strings.HasSuffix(failure.Stderr, "tail-end") {
					t.Fatal("stderr tail missing or unbounded")
				}
			}
			if mode == "timeout" && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				t.Fatalf("failure did not exercise the deadline: %v", failure)
			}
			if mode == "cancel" && ctx.Err() != nil {
				t.Fatal("consumer failure waited for the outer deadline instead of cancelling the child")
			}
		})
	}
	if _, f := runOracle(t.Context(), filepath.Join(t.TempDir(), "missing-node"), nil, os.Environ(), h); f == nil || !strings.HasPrefix(f.Diagnostic, "start oracle:") {
		t.Fatal("missing dependency did not fail")
	}
}
