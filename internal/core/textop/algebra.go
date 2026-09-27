package textop

// cursor walks an immutable operation. Every iteration consumes a full insert,
// a full independent component, or min(remainingA, remainingB) units.
type cursor struct {
	comps     []component
	i, offset int
}

func (c cursor) done() bool {
	return c.i == len(c.comps)
}
func (c cursor) kind() kind {
	return c.comps[c.i].k
}
func (c cursor) left() int {
	return c.comps[c.i].n - c.offset
}
func (c cursor) text(n int) []uint16 {
	return c.comps[c.i].u[c.offset : c.offset+n]
}
func (c *cursor) take(n int) {
	c.offset += n
	if c.offset == c.comps[c.i].n {
		c.i++
		c.offset = 0
	}
}

// Compose combines sequential a then b. Input operations must be valid in
// their document contexts; it cannot validate boundaries in unknown base text.
func Compose(a, b Op) (Op, error) {
	return DefaultLimits().Compose(a, b)
}

// Compose combines sequential operations under l's bounds.
func (l Limits) Compose(a, b Op) (Op, error) {
	if err := l.check(); err != nil {
		return Op{}, err
	}
	if err := l.admit(a); err != nil {
		return Op{}, err
	}
	if err := l.admit(b); err != nil {
		return Op{}, err
	}
	if a.target != b.base {
		return Op{}, ErrLengthMismatch
	}
	out, err := NewBuilder(l)
	if err != nil {
		return Op{}, err
	}
	x, y := cursor{comps: a.comps}, cursor{comps: b.comps}
	for !x.done() || !y.done() {
		switch {
		case !x.done() && x.kind() == deleteText:
			n := x.left()
			out.Delete(n)
			x.take(n)
		case !y.done() && y.kind() == insert:
			n := y.left()
			out.insertUnits(y.text(n))
			y.take(n)
		case x.done() || y.done():
			return Op{}, ErrLengthMismatch
		default:
			n := min(x.left(), y.left())
			if x.kind() == insert {
				// Even discarded inserted text must not be split by b.
				if !boundary(x.comps[x.i].u, x.offset+n) {
					return Op{}, ErrSplitsSurrogate
				}
			}
			switch {
			case x.kind() == retain && y.kind() == retain:
				out.Retain(n)
			case x.kind() == retain && y.kind() == deleteText:
				out.Delete(n)
			case x.kind() == insert && y.kind() == retain:
				out.insertUnits(x.text(n))
			case x.kind() == insert && y.kind() == deleteText: // insertion cancels
			}
			x.take(n)
			y.take(n)
		}
		if out.err != nil {
			return Op{}, out.err
		}
	}
	return out.Op()
}

// Transform rebases concurrent a and b with a winning insertion ties.
// ap applies after b; bp applies after a. Both inputs must be valid on the
// SAME base document, a context condition not verifiable from lengths alone.
func Transform(a, b Op) (ap, bp Op, err error) {
	return DefaultLimits().Transform(a, b)
}

// Transform rebases concurrent operations under l's bounds.
func (l Limits) Transform(a, b Op) (ap, bp Op, err error) {
	if err := l.check(); err != nil {
		return Op{}, Op{}, err
	}
	if err := l.admit(a); err != nil {
		return Op{}, Op{}, err
	}
	if err := l.admit(b); err != nil {
		return Op{}, Op{}, err
	}
	if a.base != b.base {
		return Op{}, Op{}, ErrLengthMismatch
	}
	p, err := NewBuilder(l)
	if err != nil {
		return Op{}, Op{}, err
	}
	q, err := NewBuilder(l)
	if err != nil {
		return Op{}, Op{}, err
	}
	x, y := cursor{comps: a.comps}, cursor{comps: b.comps}
	for !x.done() || !y.done() {
		switch {
		case !x.done() && x.kind() == insert:
			n := x.left()
			p.insertUnits(x.text(n))
			q.Retain(n)
			x.take(n)
		case !y.done() && y.kind() == insert:
			n := y.left()
			p.Retain(n)
			q.insertUnits(y.text(n))
			y.take(n)
		case x.done() || y.done():
			return Op{}, Op{}, ErrLengthMismatch
		default:
			n := min(x.left(), y.left())
			switch {
			case x.kind() == retain && y.kind() == retain:
				p.Retain(n)
				q.Retain(n)
			case x.kind() == deleteText && y.kind() == retain:
				p.Delete(n)
			case x.kind() == retain && y.kind() == deleteText:
				q.Delete(n)
			case x.kind() == deleteText && y.kind() == deleteText: // already deleted
			}
			x.take(n)
			y.take(n)
		}
		if p.err != nil {
			return Op{}, Op{}, p.err
		}
		if q.err != nil {
			return Op{}, Op{}, q.err
		}
	}
	ap, err = p.Op()
	if err != nil {
		return Op{}, Op{}, err
	}
	bp, err = q.Op()
	if err != nil {
		return Op{}, Op{}, err
	}
	return ap, bp, nil
}
