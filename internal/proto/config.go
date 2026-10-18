package proto

import (
	"errors"
	"fmt"

	"github.com/BarneyLaw/stele-sync/internal/core/annot"
	"github.com/BarneyLaw/stele-sync/internal/core/ids"
	"github.com/BarneyLaw/stele-sync/internal/core/textop"
)

// Subprotocol is the phase 2 WebSocket contract identifier.
const Subprotocol = "stele.v1"

// MaxDepth bounds framing containers before allocation, including unknown fields.
const MaxDepth = 64

// Subprotocols returns an owned list, newest supported version first.
func Subprotocols() []string { return []string{Subprotocol} }

// Config bounds transport work; validate it once when assembling the server.
type Config struct {
	MaxFrame, EnvelopeReserve, MaxServerFrame, MaxInline, PageEntries int
	Text                                                              textop.Limits
	Annot                                                             annot.Limits
	MaxInlineContent                                                  int
}

// DefaultConfig supplies the M4 framing and core-parser budgets.
func DefaultConfig() Config {
	c := Config{MaxFrame: 1 << 20, EnvelopeReserve: 16 << 10, MaxServerFrame: 2 << 20, MaxInline: 512 << 10, PageEntries: 500, Text: textop.DefaultLimits(), Annot: annot.DefaultLimits(), MaxInlineContent: (1 << 20) - (16 << 10)}
	c.Text.MaxJSONBytes = c.MaxInlineContent
	c.Annot.MaxOpBytes = c.MaxInlineContent
	return c
}

// Validate reports all configuration violations without overflowing arithmetic.
func (c Config) Validate() error {
	var errs []error
	for _, p := range []struct {
		name string
		n    int
	}{{"MaxFrame", c.MaxFrame}, {"EnvelopeReserve", c.EnvelopeReserve}, {"MaxServerFrame", c.MaxServerFrame}, {"MaxInline", c.MaxInline}, {"PageEntries", c.PageEntries}, {"MaxInlineContent", c.MaxInlineContent}, {"Text.MaxJSONBytes", c.Text.MaxJSONBytes}, {"Annot.MaxOpBytes", c.Annot.MaxOpBytes}} {
		if p.n <= 0 || uint64(p.n) > ids.MaxSafe {
			errs = append(errs, fmt.Errorf("proto: invalid %s", p.name))
		}
	}
	budget := c.MaxFrame - c.EnvelopeReserve
	if c.EnvelopeReserve >= c.MaxFrame {
		errs = append(errs, errors.New("proto: EnvelopeReserve must leave payload space"))
	}
	for _, p := range []struct {
		name string
		n    int
	}{{"Text.MaxJSONBytes", c.Text.MaxJSONBytes}, {"Annot.MaxOpBytes", c.Annot.MaxOpBytes}, {"MaxInlineContent", c.MaxInlineContent}} {
		if p.n > budget || c.MaxInline > p.n {
			errs = append(errs, fmt.Errorf("proto: %s must contain MaxInline and fit MaxFrame minus EnvelopeReserve", p.name))
		}
	}
	if c.MaxServerFrame < c.EnvelopeReserve || c.MaxInline > (c.MaxServerFrame-c.EnvelopeReserve)/2 {
		errs = append(errs, errors.New("proto: MaxServerFrame must fit two inline payloads and EnvelopeReserve"))
	}
	if _, e := c.Text.Parse([]byte("[]")); e != nil {
		errs = append(errs, fmt.Errorf("proto: Text: %w", e))
	}
	if e := c.Annot.Validate(); e != nil {
		errs = append(errs, fmt.Errorf("proto: Annot: %w", e))
	}
	return errors.Join(errs...)
}
