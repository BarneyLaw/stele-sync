package classify

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/BarneyLaw/stele-sync/internal/core/annot"
	"github.com/BarneyLaw/stele-sync/internal/core/vpath"
)

// Class is fixed at file creation; content confirmation can only demote to C.
type Class byte

const (
	// Text is UTF-8 without NUL, merged by text operations.
	Text Class = 'A'
	// Sidecar is a recognized PDF FreeDraw sidecar, merged by annotation operations.
	Sidecar Class = 'B'
	// Opaque is stored as an indivisible blob.
	Opaque Class = 'C'
)

// Demotion records why content cannot retain its expected class.
type Demotion uint8

const (
	// Kept denotes no content demotion.
	Kept Demotion = iota
	// TooLarge denotes an exceeded byte ceiling.
	TooLarge
	// InvalidUTF8 denotes malformed text bytes.
	InvalidUTF8
	// ContainsNUL denotes text containing a NUL byte.
	ContainsNUL
	// NotFreedraw denotes content rejected by the pinned annotation parser.
	NotFreedraw
)

// String returns a stable cross-language name.
func (d Demotion) String() string {
	switch d {
	case Kept:
		return "kept"
	case TooLarge:
		return "too_large"
	case InvalidUTF8:
		return "invalid_utf8"
	case ContainsNUL:
		return "contains_nul"
	case NotFreedraw:
		return "not_freedraw"
	}
	return "unknown"
}

// ErrInvalidConfig denotes an inconsistent classification configuration.
var ErrInvalidConfig = errors.New("classify: invalid config")

// Config is validated once by the shell and shared for metadata and content checks.
type Config struct {
	TextExts            []string
	MaxText, MaxSidecar int
	Annot               annot.Limits
}

// DefaultConfig uses the agreed text extensions; code extensions remain opt-in.
func DefaultConfig() Config {
	return Config{[]string{"md", "txt", "mdx"}, 8 << 20, 16 << 20, annot.DefaultLimits()}
}

// Validate checks all budgets and unambiguous lowercase ASCII extensions.
func (c Config) Validate() error {
	var errs []error
	if c.MaxText <= 0 || c.MaxSidecar <= 0 || c.Annot.MaxBytes != c.MaxSidecar {
		errs = append(errs, fmt.Errorf("%w: byte limits", ErrInvalidConfig))
	}
	if e := c.Annot.Validate(); e != nil {
		errs = append(errs, fmt.Errorf("%w: %w", ErrInvalidConfig, e))
	}
	seen := map[string]bool{}
	for _, e := range c.TextExts {
		valid := e != "" && !seen[e]
		for _, r := range e {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
				valid = false
			}
		}
		if !valid {
			errs = append(errs, fmt.Errorf("%w: extension %q", ErrInvalidConfig, e))
		}
		seen[e] = true
	}
	return errors.Join(errs...)
}

// IsSidecarName recognizes a nonempty PDF stem, using ASCII case-insensitivity.
func IsSidecarName(p vpath.Path) bool {
	b := p.Base()
	const suffix = ".pdf.annot.json"
	return len(b) > len(suffix) && asciiLower(b[len(b)-len(suffix):]) == suffix
}
func asciiLower(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, s)
}

// Expect uses metadata only. Opaque is final, and sidecars take precedence over text.
func Expect(p vpath.Path, size int64, cfg Config) Class {
	if size < 0 || p.IsZero() {
		return Opaque
	}
	if IsSidecarName(p) {
		if size <= int64(cfg.MaxSidecar) {
			return Sidecar
		}
		return Opaque
	}
	// Recovery/conflict sidecars remain opaque even when json is a text extension.
	if strings.HasSuffix(asciiLower(p.Base()), ".annot.json") {
		return Opaque
	}
	for _, e := range cfg.TextExts {
		if p.Ext() == e && size <= int64(cfg.MaxText) {
			return Text
		}
	}
	return Opaque
}

// CheckText validates the entire content, retaining BOMs and line endings.
func CheckText(b []byte, cfg Config) Demotion {
	if len(b) > cfg.MaxText {
		return TooLarge
	}
	if !utf8.Valid(b) {
		return InvalidUTF8
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return ContainsNUL
	}
	return Kept
}

// CheckSidecar delegates recognition and every annotation budget to M2.
func CheckSidecar(b []byte, cfg Config) Demotion {
	if len(b) > cfg.MaxSidecar {
		return TooLarge
	}
	if _, e := annot.Parse(b, cfg.Annot); e != nil {
		return NotFreedraw
	}
	return Kept
}

// Confirm validates complete content. Config must have passed Validate.
func Confirm(expected Class, b []byte, cfg Config) (Class, Demotion) {
	d := Kept
	switch expected {
	case Text:
		d = CheckText(b, cfg)
	case Sidecar:
		d = CheckSidecar(b, cfg)
	case Opaque:
		return Opaque, Kept
	default:
		return Opaque, Kept
	}
	if d != Kept {
		return Opaque, d
	}
	return expected, d
}

// Classify combines metadata classification with complete-content confirmation.
func Classify(p vpath.Path, b []byte, cfg Config) (Class, Demotion) {
	return Confirm(Expect(p, int64(len(b)), cfg), b, cfg)
}
