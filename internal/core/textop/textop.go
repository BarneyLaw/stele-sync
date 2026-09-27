package textop

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidUTF8 indicates malformed input text or JSON bytes.
	ErrInvalidUTF8 = errors.New("invalid UTF-8")
	// ErrInvalidJSON indicates malformed JSON or a non-array operation.
	ErrInvalidJSON = errors.New("invalid operation JSON")
	// ErrInvalidCount indicates an unsupported count or component type.
	ErrInvalidCount = errors.New("invalid count")
	// ErrNonCanonical indicates redundant or incorrectly ordered components.
	ErrNonCanonical = errors.New("non-canonical operation")
	// ErrLengthMismatch indicates incompatible operation/document lengths.
	ErrLengthMismatch = errors.New("length mismatch")
	// ErrSplitsSurrogate indicates a boundary inside a surrogate pair.
	ErrSplitsSurrogate = errors.New("boundary splits surrogate pair")
	// ErrInvalidUnicode indicates an unpaired surrogate in a JSON string.
	ErrInvalidUnicode = errors.New("invalid Unicode escape")
	// ErrLimit indicates an operation or document exceeds its configured budget.
	ErrLimit = errors.New("textop limit exceeded")
	// ErrInvalidLimits indicates a nonpositive or non-interoperable policy.
	ErrInvalidLimits = errors.New("invalid textop limits")
)

// Limits bounds allocation and work. Methods on a Limits value use that policy;
// package functions and a zero Builder use DefaultLimits(). No globals mutate.
type Limits struct {
	MaxUnits      int // maximum base or target UTF-16 length
	MaxDocBytes   int // maximum materialized UTF-8 document size
	MaxComponents int // maximum canonical operation components
	MaxJSONBytes  int // maximum encoded input to Parse
}

// DefaultLimits returns the default policy. The component ceiling is an M1
// proposal; the document and frame byte ceilings follow the architecture.
func DefaultLimits() Limits {
	return Limits{
		MaxUnits:      8 << 20, // 8,388,608 UTF-16 units
		MaxDocBytes:   8 << 20, // 8 MiB UTF-8
		MaxComponents: 65536,   // 2^16
		MaxJSONBytes:  1 << 20, // 1 MiB encoded ingress
	}
}

func (l Limits) check() error {
	if l.MaxUnits <= 0 || l.MaxDocBytes <= 0 || l.MaxComponents <= 0 || l.MaxJSONBytes <= 0 {
		return ErrInvalidLimits
	}
	// Counts must be exactly representable by JavaScript and by the local int.
	if uint64(l.MaxUnits) > (uint64(1)<<53)-1 {
		return ErrInvalidLimits
	}
	return nil
}

// ValidateAgainst checks applicability and output size without allocating an
// output document. The caller must supply the operation's actual base document.
func ValidateAgainst(d Doc, o Op) error {
	return DefaultLimits().ValidateAgainst(d, o)
}

// ValidateAgainst checks applicability using l's bounds.
func (l Limits) ValidateAgainst(d Doc, o Op) error {
	if err := l.check(); err != nil {
		return err
	}
	if err := l.admit(o); err != nil {
		return err
	}
	if o.base != len(d.u) {
		return ErrLengthMismatch
	}
	if _, err := countBytes(d.u, l.MaxDocBytes); err != nil {
		return err
	}
	p, size := 0, 0
	for i, c := range o.comps {
		var copied []uint16
		if c.k == insert {
			copied = c.u
		} else {
			next := p + c.n // canonical op lengths were bounded during construction
			if !boundary(d.u, next) {
				return fmt.Errorf("component %d: %w", i, ErrSplitsSurrogate)
			}
			if c.k == retain {
				copied = d.u[p:next]
			}
			p = next
		}
		// Every insertion position is a preceding retain/delete endpoint,
		// or zero. Those endpoints were checked even if their text was deleted.
		n, err := countBytes(copied, l.MaxDocBytes-size)
		if err != nil {
			return err
		}
		size += n
	}
	return nil
}

// Apply returns a new document, preserving d and o.
func Apply(d Doc, o Op) (Doc, error) {
	return DefaultLimits().Apply(d, o)
}

// Apply materializes an operation under l's bounds.
func (l Limits) Apply(d Doc, o Op) (Doc, error) {
	if err := l.ValidateAgainst(d, o); err != nil {
		return Doc{}, err
	}
	out := make([]uint16, 0, o.target)
	p := 0
	for _, c := range o.comps {
		switch c.k {
		case retain:
			out = append(out, d.u[p:p+c.n]...)
			p += c.n
		case deleteText:
			p += c.n
		case insert:
			out = append(out, c.u...)
		}
	}
	return Doc{u: out}, nil
}
