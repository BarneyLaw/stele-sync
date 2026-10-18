package proto

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	if e := DefaultConfig().Validate(); e != nil {
		t.Fatal(e)
	}
	for _, change := range []func(*Config){func(c *Config) { c.MaxFrame = 0 }, func(c *Config) { c.EnvelopeReserve = 0 }, func(c *Config) { c.MaxServerFrame = 1 }, func(c *Config) { c.MaxInline = 0 }, func(c *Config) { c.PageEntries = 0 }, func(c *Config) { c.Text.MaxJSONBytes = c.MaxFrame }, func(c *Config) { c.Annot.MaxOpBytes = c.MaxFrame }, func(c *Config) { c.MaxInlineContent = c.MaxFrame }, func(c *Config) { c.Text.MaxUnits = 0 }, func(c *Config) { c.Annot.MaxBytes = 0 }, func(c *Config) { c.MaxInline = c.MaxFrame }} {
		c := DefaultConfig()
		change(&c)
		if c.Validate() == nil {
			t.Fatal(c)
		}
	}
	c := Config{}
	e := c.Validate()
	if e == nil || !strings.Contains(e.Error(), "MaxFrame") || !strings.Contains(e.Error(), "PageEntries") {
		t.Fatal(e)
	}
}
func TestTokenRedaction(t *testing.T) {
	secret := "stele_" + strings.Repeat("A", 43)
	token, e := ParseToken(secret)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range []any{token, &token, Hello{Token: token}, &Hello{Token: token}} {
		for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%#x"} {
			s := fmt.Sprintf(format, v)
			if strings.Contains(s, strings.Repeat("A", 43)) {
				t.Fatal("token leaked", format)
			}
		}
	}
	for _, jsonLog := range []bool{false, true} {
		var buf bytes.Buffer
		var handler slog.Handler = slog.NewTextHandler(&buf, nil)
		if jsonLog {
			handler = slog.NewJSONHandler(&buf, nil)
		}
		slog.New(handler).Info("test", "token", token, "hello", Hello{Token: token}, "error", errors.New(fmt.Sprint(Hello{Token: token})))
		if strings.Contains(buf.String(), strings.Repeat("A", 43)) {
			t.Fatal("log leaked token")
		}
	}
	if token.String() != "stele_[redacted]" || token.GoString() != token.String() || token.Hash() == [32]byte{} {
		t.Fatal("token methods")
	}
	for _, s := range []string{"", secret + "A", secret[:48], "obs_" + strings.Repeat("A", 43), "stele_" + strings.Repeat("+", 43)} {
		if _, e := ParseToken(s); e == nil {
			t.Fatal("bad token accepted")
		}
	}
}
