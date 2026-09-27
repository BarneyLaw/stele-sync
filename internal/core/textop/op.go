package textop

type kind uint8

// These are all the kinds of operations that exist
const (
	retain kind = iota
	insert
	deleteText
)

// component A component
type component struct {
	k kind
	n int
	u []uint16 // Only insert carries text; n equals len(u).
}

// Op is a canonical immutable operation. The zero value is [] and has base 0.
type Op struct {
	comps        []component
	base, target int
}

// BaseLen returns how many UTF-16 units op consumes
func (op Op) BaseLen() int {
	return op.base
}

// TargetLen returns how many UTF-16 units op produces
func (op Op) TargetLen() int {
	return op.target
}

// admit returns an error if the op goes beyond our specified limits
func (l Limits) admit(op Op) error {
	if op.base > l.MaxUnits || op.target > l.MaxUnits || len(op.comps) > l.MaxComponents {
		return ErrLimit
	}
	return nil
}
