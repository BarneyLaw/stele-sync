package vpath

import (
	"crypto/sha256"
	"fmt"
	"testing"
	"unicode/utf8"
)

// Keep published phase 1 spelling/collision behavior when upgrading Go and
// x/text. These inputs gained normalization or casing after Unicode 15.
func TestUnicodeCompatibility(t *testing.T) {
	for _, s := range []string{"\U000105d2\u0307.md", "a\u0315\u0897\u0300", "\u1c89.md", "\ua7cb.md", "\U00010d50.md", "\U00016ea0.md"} {
		if got := CollisionKey(s); got != s {
			t.Errorf("legacy collision spelling changed: %q -> %q", s, got)
		}
		if got, err := Component(s); err != nil || got != s {
			t.Errorf("legacy component changed: %q -> %q (%v)", s, got, err)
		}
		if _, err := Parse(s); err != nil {
			t.Errorf("wire path requires a newer client Unicode version: %q: %v", s, err)
		}
	}
}

// Hash all scalar collision keys with NUL separators. The digest was measured
// with Go 1.26.6 / Unicode 15 before upgrading, independently of the compatibility
// implementation. Supplement with the multi-scalar cases/normalization contract.
func TestLegacyScalarKeys(t *testing.T) {
	h := sha256.New()
	for r := rune(0); r <= utf8.MaxRune; r++ {
		if utf8.ValidRune(r) {
			_, _ = h.Write([]byte(CollisionKey(string(r))))
			_, _ = h.Write([]byte{0})
		}
	}
	const want = "03362113a2efd5ac944e0fcb198d64b6da42f3a0dcca6f297e052f0b46ce6844"
	if got := fmt.Sprintf("%x", h.Sum(nil)); got != want {
		t.Fatalf("legacy scalar keys changed: %s != %s", got, want)
	}
}
