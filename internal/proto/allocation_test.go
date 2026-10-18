package proto

import (
	"runtime"
	"strings"
	"testing"
)

func TestPayloadAllocationBound(t *testing.T) {
	c := DefaultConfig()
	for _, raw := range []string{
		submitPrefix + `"kind":"text_edit","file_id":"` + testID + `","base_version":1,"op":["` + strings.Repeat("a", c.Text.MaxJSONBytes-4) + `"]}`,
		submitPrefix + `"kind":"annot_edit","file_id":"` + testID + `","base_version":1,"ops":` + string(mustJSON(t, `{"meta":{"put":{"x":"`+strings.Repeat("a", c.Annot.MaxOpBytes-256)+`"}}}`)) + `}`,
	} {
		frame := []byte(raw)
		if _, e := DecodeClient(frame, c); e != nil {
			t.Fatal(e)
		} // Warm library caches outside the measurement.
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		if _, e := DecodeClient(frame, c); e != nil {
			t.Fatal(e)
		}
		runtime.ReadMemStats(&after)
		allocated := after.TotalAlloc - before.TotalAlloc
		t.Logf("frame=%d allocated=%d amplification=%.2f", len(frame), allocated, float64(allocated)/float64(len(frame)))
		// This is cumulative allocation, including core validation and canonical
		// encoding passes, not simultaneous resident memory. Keep the largest
		// representative payloads below the configured 64 MiB state budget.
		if allocated > 64<<20 {
			t.Fatal("decoder cumulative allocation exceeded the state budget")
		}
	}
}
