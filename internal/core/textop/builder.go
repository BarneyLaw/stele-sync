package textop

import "unicode/utf16"

// Builder constructs canonical operations and retains its first error.
// It is not safe for concurrent use. Do not copy a non-zero Builder.
// Finalized operations do not alias it.
type Builder struct {
	limits       Limits
	configured   bool
	comps        []component
	base, target int
	err          error
}

// NewBuilder returns a builder using l rather than the default limits.
func NewBuilder(l Limits) (*Builder, error) {
	if err := l.check(); err != nil {
		return nil, err
	}
	return &Builder{limits: l, configured: true}, nil
}

// initLimits initializes the limit for the operations to come
func (b *Builder) initLimits() {
	if !b.configured {
		b.limits = DefaultLimits()
		b.configured = true
	}
}

func (b *Builder) grow(base, target int) bool {
	b.initLimits()
	if b.err != nil {
		return false
	}
	if base < 0 || target < 0 {
		b.err = ErrInvalidCount
		return false
	}
	if base > b.limits.MaxUnits-b.base || target > b.limits.MaxUnits-b.target {
		b.err = ErrLimit
		return false
	}
	b.base += base
	b.target += target
	return true
}

func (b *Builder) append(c component) {
	if len(b.comps) >= b.limits.MaxComponents {
		b.err = ErrLimit
		return
	}
	b.comps = append(b.comps, c)
}

// insertUnits receives only well-formed UTF-16 from package internals.
// append copies into builder-owned buffers, never into an input operation.
func (b *Builder) insertUnits(u []uint16) {
	if !b.grow(0, len(u)) || len(u) == 0 {
		return
	}
	i := len(b.comps) - 1
	if i >= 0 && b.comps[i].k == deleteText {
		i--
	}
	if i >= 0 && b.comps[i].k == insert {
		b.comps[i].u = append(b.comps[i].u, u...)
		b.comps[i].n += len(u)
		return
	}
	c := component{k: insert, n: len(u), u: append([]uint16(nil), u...)}
	last := len(b.comps) - 1
	if last >= 0 && b.comps[last].k == deleteText {
		d := b.comps[last]
		b.append(d)
		if b.err == nil {
			b.comps[last] = c
		}
	} else {
		b.append(c)
	}
}

// Retain preserves n input units. Zero is ignored; negative n is an error.
func (b *Builder) Retain(n int) *Builder {
	if !b.grow(n, n) || n == 0 {
		return b
	}
	i := len(b.comps) - 1
	if i >= 0 && b.comps[i].k == retain {
		b.comps[i].n += n
	} else {
		b.append(component{k: retain, n: n})
	}
	return b
}

// Insert adds valid UTF-8. Empty text is ignored. Text is never normalized.
func (b *Builder) Insert(s string) *Builder {
	b.initLimits()
	if b.err != nil {
		return b
	}
	n, err := unitsIn(s, b.limits.MaxUnits-b.target)
	if err != nil {
		b.err = err
		return b
	}
	u := make([]uint16, 0, n)
	for _, r := range s {
		u = utf16.AppendRune(u, r)
	}
	b.insertUnits(u)
	return b
}

// Delete consumes n input units without retaining them.
func (b *Builder) Delete(n int) *Builder {
	if !b.grow(n, 0) || n == 0 {
		return b
	}
	i := len(b.comps) - 1
	if i >= 0 && b.comps[i].k == deleteText {
		b.comps[i].n += n
	} else {
		b.append(component{k: deleteText, n: n})
	}
	return b
}

// Op returns an independent immutable snapshot, or the first builder error.
// A builder can continue after a successful call; errors are sticky.
func (b *Builder) Op() (Op, error) {
	b.initLimits()
	if b.err != nil {
		return Op{}, b.err
	}
	cs := make([]component, len(b.comps))
	copy(cs, b.comps)
	for i := range cs {
		if cs[i].k == insert {
			cs[i].u = append([]uint16(nil), cs[i].u...)
		}
	}
	return Op{comps: cs, base: b.base, target: b.target}, nil
}
