package rules

import (
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/BarneyLaw/stele-sync/internal/core/vpath"
)

// MarkerKind chooses numbering or an explicit conflict archive.
type MarkerKind uint8

const (
	// Numbered continues any existing numeric suffix.
	Numbered MarkerKind = iota
	// Conflict always marks the copy, even when the original name is free.
	Conflict
)

// Marker contains already-localized display time; core never loads a timezone.
type Marker struct {
	Kind   MarkerKind
	Device string
	At     time.Time
}

func isPDF(p vpath.Path) bool {
	b := p.Base()
	return len(b) > 4 && strings.EqualFold(b[len(b)-4:], ".pdf")
}

// SidecarOf appends the FreeDraw suffix, preserving the exact PDF spelling.
func SidecarOf(pdf vpath.Path) (vpath.Path, error) {
	if !isPDF(pdf) {
		return vpath.Path{}, ErrNotPDF
	}
	p, e := pdf.WithBase(pdf.Base() + ".annot.json")
	if e != nil {
		return vpath.Path{}, fmt.Errorf("%w: %w", ErrNameTooLong, e)
	}
	return p, nil
}

// PDFOf reverses the sidecar suffix only for a nonempty PDF stem.
func PDFOf(sidecar vpath.Path) (vpath.Path, bool) {
	b := sidecar.Base()
	const suffix = ".annot.json"
	if len(b) <= len(suffix) || !strings.EqualFold(b[len(b)-len(suffix):], suffix) {
		return vpath.Path{}, false
	}
	p, e := sidecar.WithBase(b[:len(b)-len(suffix)])
	return p, e == nil && isPDF(p)
}

// GroupTaken reserves both names together. The shell supplies a locked key lookup.
// FreeName reserves suffix space when numbering PDFs, so maximum-length pairs fit.
func GroupTaken(taken func(vpath.Path) (bool, error)) func(vpath.Path) (bool, error) {
	return func(p vpath.Path) (bool, error) {
		busy, e := taken(p)
		if busy || e != nil {
			return busy, e
		}
		sc, e := SidecarOf(p)
		if e != nil {
			return false, e
		}
		return taken(sc)
	}
}
func splitName(base string, compound []string) (string, string) {
	ext := ""
	for _, s := range compound {
		if len(s) > len(ext) && len(base) > len(s) && strings.EqualFold(base[len(base)-len(s):], s) {
			ext = base[len(base)-len(s):]
		}
	}
	if ext != "" {
		return base[:len(base)-len(ext)], ext
	}
	i := strings.LastIndexByte(base, '.')
	if i > 0 && len(base)-i-1 <= 16 {
		return base[:i], base[i:]
	}
	return base, ""
}
func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
func nextNumber(stem string) (string, *big.Int) {
	n := big.NewInt(2)
	if !strings.HasSuffix(stem, ")") {
		return stem, n
	}
	i := strings.LastIndex(stem, " (")
	if i <= 0 {
		return stem, n
	}
	digits := stem[i+2 : len(stem)-1]
	for _, r := range digits {
		if r < '0' || r > '9' {
			return stem, n
		}
	}
	k, ok := new(big.Int).SetString(digits, 10)
	if !ok || k.Cmp(n) < 0 {
		return stem, n
	}
	return stem[:i], k.Add(k, big.NewInt(1))
}

// FreeName returns the first available marked candidate, bounded by MaxAttempts.
// Callback errors are returned unchanged. Validate Config before using decisions.
func FreeName(p vpath.Path, m Marker, cfg Config, taken func(vpath.Path) (bool, error)) (vpath.Path, error) {
	if e := cfg.Validate(); e != nil {
		return vpath.Path{}, e
	}
	if p.IsZero() || taken == nil || m.Kind > Conflict {
		return vpath.Path{}, ErrInvalidConfig
	}
	stem, ext := splitName(p.Base(), cfg.Compound)
	number := big.NewInt(2)
	if m.Kind == Numbered {
		stem, number = nextNumber(stem)
	}
	device, e := vpath.Component(m.Device)
	if e != nil {
		device = "device"
	}
	device = strings.TrimRight(cut(vpath.NFC(device), 32), ". ")
	if device == "" {
		device = "device"
	}
	for i := 0; i < cfg.MaxAttempts; i++ {
		var marker string
		switch m.Kind {
		case Numbered:
			marker = " (" + number.String() + ")"
			number.Add(number, big.NewInt(1))
		case Conflict:
			tail := ""
			if i > 0 {
				tail = fmt.Sprintf(" %d", i+1)
			}
			marker = " (conflict " + m.At.Format("2006-01-02 1504") + " " + device + tail + ")"
		}
		budget := vpath.DeviceMaxComponent - len(marker) - len(ext)
		if m.Kind == Numbered && isPDF(p) {
			budget -= len(".annot.json")
		}
		if budget < 1 {
			return vpath.Path{}, ErrNameTooLong
		}
		s := strings.TrimRight(cut(stem, budget), ". ")
		if s == "" {
			s = "_"
		}
		candidate, e := p.WithBase(s + marker + ext)
		if e != nil {
			return vpath.Path{}, fmt.Errorf("%w: %w", ErrNameTooLong, e)
		}
		busy, e := taken(candidate)
		if e != nil {
			return vpath.Path{}, e
		}
		if !busy {
			return candidate, nil
		}
	}
	return vpath.Path{}, ErrNoFreeName
}
