// Command proto-probe records the standard-library assumptions behind M4.
package main

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
)

func main() {
	_, _ = fmt.Fprintln(os.Stdout, runtime.Version())
	for _, s := range []string{`"\ud800"`, `"\udc00"`, `"\ud800A"`, `"\udc00\ud800"`, `"\ud800\udc00"`} {
		_, err := jsontext.NewDecoder(strings.NewReader(s)).ReadValue()
		_, _ = fmt.Fprintf(os.Stdout, "string %s: %v\n", s, err)
	}
	for _, s := range []string{"1.0", "1e3", "-0", "01", "9007199254740992", "9223372036854775808", "null"} {
		var v int64
		err := json.Unmarshal([]byte(s), &v)
		_, _ = fmt.Fprintf(os.Stdout, "integer %s: value=%d error=%v\n", s, v, err)
	}
	for _, s := range []string{`{"extra":1}`, `{"Value":1}`, `{"nested":{"Value":1}}`, `{"nested":{"extra":1}}`} {
		var v struct {
			Value  *int `json:"value"`
			Nested *struct {
				Value *int `json:"value"`
			} `json:"nested"`
		}
		_, _ = fmt.Fprintf(os.Stdout, "unknown %s: %v\n", s, json.Unmarshal([]byte(s), &v, json.RejectUnknownMembers(true)))
	}
	inputs := map[string][]byte{
		"1MiB string":          []byte(`"` + strings.Repeat("a", (1<<20)-2) + `"`),
		"65536 integers":       []byte(`[` + strings.Repeat("1,", 65535) + `1]`),
		"10000 nested objects": []byte(strings.Repeat(`{"a":`, 10000) + `0` + strings.Repeat(`}`, 10000)),
	}
	for name, raw := range inputs {
		var last error
		result := testing.Benchmark(func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, last = jsontext.NewDecoder(bytes.NewReader(raw)).ReadValue()
			}
		})
		_, _ = fmt.Fprintf(os.Stdout, "%s: input=%d B/op=%d allocs/op=%d error=%v\n", name, len(raw), result.AllocedBytesPerOp(), result.AllocsPerOp(), last)
	}
}
