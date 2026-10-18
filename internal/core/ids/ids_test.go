package ids_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/BarneyLaw/stele-sync/internal/core/ids"
	"pgregory.net/rapid"
)

func TestPropIDRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ms := rapid.Int64Range(0, 1<<48-1).Draw(t, "milliseconds")
		entropy := rapid.SliceOfN(rapid.Byte(), 10, 10).Draw(t, "entropy")
		f, err := ids.NewFileID(ms, bytes.NewReader(entropy))
		if err != nil {
			t.Fatal(err)
		}
		p, err := ids.ParseFileID(f.String())
		if err != nil || p != f {
			t.Fatalf("file round trip: %v", err)
		}
		d, err := ids.NewDeviceID(ms, bytes.NewReader(entropy))
		if err != nil || d.String() != f.String() {
			t.Fatal("device generation", err)
		}
		o, err := ids.NewClientOpID(ms, bytes.NewReader(entropy))
		if err != nil || o.String() != f.String() {
			t.Fatal("operation generation", err)
		}
		for _, v := range []any{f, d, o} {
			raw, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			var f2 ids.FileID
			var d2 ids.DeviceID
			var o2 ids.ClientOpID
			if json.Unmarshal(raw, &f2) != nil || json.Unmarshal(raw, &d2) != nil || json.Unmarshal(raw, &o2) != nil || f2 != f || d2 != d || o2 != o {
				t.Fatal("JSON round trip")
			}
		}
	})
}

func TestIDCanonical(t *testing.T) {
	good := "019a1b2c-0000-7000-8000-000000000abc"
	for _, s := range []string{"", strings.ToUpper(good), "{" + good + "}", strings.ReplaceAll(good, "-", ""), strings.Replace(good, "-7000", "-4000", 1), strings.Replace(good, "-8000", "-c000", 1), strings.Replace(good, "abc", "abg", 1), "00000000-0000-0000-0000-000000000000"} {
		if _, e := ids.ParseFileID(s); !errors.Is(e, ids.ErrInvalidID) {
			t.Errorf("file accepted %q: %v", s, e)
		}
		if _, e := ids.ParseDeviceID(s); !errors.Is(e, ids.ErrInvalidID) {
			t.Errorf("device accepted %q", s)
		}
		if _, e := ids.ParseClientOpID(s); !errors.Is(e, ids.ErrInvalidID) {
			t.Errorf("op accepted %q", s)
		}
		var f ids.FileID
		var d ids.DeviceID
		var o ids.ClientOpID
		if f.UnmarshalText([]byte(s)) == nil || d.UnmarshalText([]byte(s)) == nil || o.UnmarshalText([]byte(s)) == nil {
			t.Fatal("invalid text accepted")
		}
	}
	for _, ms := range []int64{-1, 1 << 48} {
		if _, e := ids.NewFileID(ms, bytes.NewReader(make([]byte, 10))); !errors.Is(e, ids.ErrInvalidID) {
			t.Fatal(e)
		}
	}
	if _, e := ids.NewFileID(0, bytes.NewReader(nil)); !errors.Is(e, io.EOF) {
		t.Fatal(e)
	}
	if _, e := ids.NewDeviceID(0, nil); e == nil {
		t.Fatal("nil entropy")
	}
	if _, e := ids.NewClientOpID(-1, nil); e == nil {
		t.Fatal("bad time")
	}
	for _, v := range []interface{ MarshalText() ([]byte, error) }{ids.FileID{}, ids.DeviceID{}, ids.ClientOpID{}} {
		if _, e := v.MarshalText(); !errors.Is(e, ids.ErrInvalidID) {
			t.Fatal("zero marshaled", e)
		}
	}
}

func TestSafeIntegers(t *testing.T) {
	for _, n := range []int64{-1, 0, 1, ids.MaxSafe, ids.MaxSafe + 1} {
		if ids.Seq(n).Valid() != (n >= 0 && n <= ids.MaxSafe) || ids.Version(n).Valid() != (n >= 1 && n <= ids.MaxSafe) {
			t.Fatal(n)
		}
	}
}

func FuzzParseIDs(f *testing.F) {
	f.Add("019a1b2c-0000-7000-8000-000000000abc")
	f.Add("")
	f.Fuzz(func(t *testing.T, s string) {
		id, e := ids.ParseFileID(s)
		if e != nil {
			return
		}
		if id.String() != s {
			t.Fatal("noncanonical accepted")
		}
		b, e := id.MarshalText()
		if e != nil || string(b) != s {
			t.Fatal(e)
		}
		if _, e := ids.ParseDeviceID(s); e != nil {
			t.Fatal(e)
		}
		if _, e := ids.ParseClientOpID(s); e != nil {
			t.Fatal(e)
		}
	})
}
