package textop

import "unicode/utf8"

// unitsIn checks how many UTF-16 code units are needed to represent a Go string, while enforcing a maximum limit.
func unitsIn(s string, limit int) (int, error) {
	if !utf8.ValidString(s) {
		return 0, ErrInvalidUTF8
	}
	n := 0
	for _, r := range s {
		width := 1
		if r > 0xffff {
			width = 2
		}
		if width > limit-n {
			return 0, ErrLimit
		}
		n += width
	}
	return n, nil
}

// boundary checks if checks whether a position p is a safe place to split a UTF-16 string
func boundary(u []uint16, p int) bool {
	return p == 0 || p == len(u) || u[p-1] < 0xd800 || u[p-1] > 0xdbff || u[p] < 0xdc00 || u[p] > 0xdfff
}

// countBytes counts well-formed UTF-16's UTF-8 size, stopping at the budget.
func countBytes(u []uint16, budget int) (int, error) {
	n := 0
	for i := 0; i < len(u); i++ {
		width := 3
		switch {
		case u[i] < 0x80:
			width = 1
		case u[i] < 0x800:
			width = 2
		case u[i] >= 0xd800 && u[i] <= 0xdbff:
			width = 4
			i++
		}
		if width > budget-n {
			return 0, ErrLimit
		}
		n += width
	}
	return n, nil
}
