package vpath

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// DeviceMaxComponent bounds UTF-8 device components, leaving space for markers.
const DeviceMaxComponent = 200

// DeviceMaxTotal bounds a relative wire path in UTF-8 bytes.
const DeviceMaxTotal = 512

// Path is a validated NFC relative vault path. Its zero value is not a file.
type Path struct{ s string }

// Key is a server-computed uniqueness key; it is never a display path.
type Key string

// Reason is a stable parse failure category.
type Reason uint8

const (
	// ReasonNone denotes success.
	ReasonNone Reason = iota
	// InvalidUTF8 denotes malformed input bytes.
	InvalidUTF8
	// NotNFC denotes a noncanonical spelling.
	NotNFC
	// Empty denotes an empty path.
	Empty
	// Absolute denotes a leading slash.
	Absolute
	// Backslash denotes an unsupported separator.
	Backslash
	// EmptyComponent denotes repeated or trailing separators.
	EmptyComponent
	// DotComponent denotes a current-directory component.
	DotComponent
	// Traversal denotes a parent-directory component.
	Traversal
	// IllegalChar denotes a Windows-reserved character.
	IllegalChar
	// ControlChar denotes a Unicode control character.
	ControlChar
	// LeadingSpace denotes a component beginning with ASCII space.
	LeadingSpace
	// TrailingDotOrSpace denotes an unstable Windows suffix.
	TrailingDotOrSpace
	// ReservedName denotes a Windows device name.
	ReservedName
	// ComponentTooLong denotes the per-component byte ceiling.
	ComponentTooLong
	// PathTooLong denotes the total path byte ceiling.
	PathTooLong
)

// String returns the stable snake_case reason used at protocol boundaries.
func (r Reason) String() string {
	names := [...]string{"none", "invalid_utf8", "not_nfc", "empty", "absolute", "backslash", "empty_component", "dot_component", "traversal", "illegal_char", "control_char", "leading_space", "trailing_dot_or_space", "reserved_name", "component_too_long", "path_too_long"}
	if int(r) >= len(names) {
		return "unknown"
	}
	return names[r]
}

// PathError locates a parse failure. Component is zero-based, or -1 for the path.
type PathError struct {
	Input     string
	Component int
	Reason    Reason
}

// Error describes the failure without changing its stable reason.
func (e *PathError) Error() string {
	return fmt.Sprintf("vpath: %s at component %d in %q", e.Reason, e.Component, e.Input)
}

// Parse checks rules in phases, so structural errors precede character/size errors.
func Parse(s string) (Path, error) {
	fail := func(r Reason, i int) (Path, error) { return Path{}, &PathError{s, i, r} }
	switch {
	case !utf8.ValidString(s):
		return fail(InvalidUTF8, -1)
	case !norm.NFC.IsNormalString(s):
		return fail(NotNFC, -1)
	case s == "":
		return fail(Empty, -1)
	case strings.HasPrefix(s, "/"):
		return fail(Absolute, -1)
	case strings.Contains(s, `\`):
		return fail(Backslash, -1)
	}
	parts := strings.Split(s, "/")
	for i, c := range parts {
		switch c {
		case "":
			return fail(EmptyComponent, i)
		case ".":
			return fail(DotComponent, i)
		case "..":
			return fail(Traversal, i)
		}
	}
	for i, c := range parts {
		if r := characterReason(c); r != ReasonNone {
			return fail(r, i)
		}
	}
	for i, c := range parts {
		if len(c) > DeviceMaxComponent {
			return fail(ComponentTooLong, i)
		}
	}
	if len(s) > DeviceMaxTotal {
		return fail(PathTooLong, -1)
	}
	return Path{s}, nil
}
func characterReason(s string) Reason {
	if characterPass(s) != s {
		if strings.ContainsAny(s, illegal) {
			return IllegalChar
		}
		for _, r := range s {
			if unicode.IsControl(r) {
				return ControlChar
			}
		}
		if strings.HasPrefix(s, " ") {
			return LeadingSpace
		}
		return TrailingDotOrSpace
	}
	if deviceReservedName(s) {
		return ReservedName
	}
	return ReasonNone
}

// The strict API also excludes Microsoft's superscript device aliases, which
// phase 1 omitted. Legacy sanitization remains byte-for-byte compatible.
func deviceReservedName(s string) bool {
	s = strings.TrimRight(stem(s), " ")
	if reservedName(s) {
		return true
	}
	switch strings.ToUpper(stem(s)) {
	case "COM¹", "COM²", "COM³", "LPT¹", "LPT²", "LPT³", "CONIN$", "CONOUT$":
		return true
	}
	return false
}

// Sanitize applies frozen Canvas sanitization then strict device validation.
// Legacy edge cases that remain nonportable are returned to the bridge for fallback.
func Sanitize(s string) (Path, error) {
	p, e := SanitizePath(s)
	if e != nil {
		return Path{}, e
	}
	return Parse(p)
}

// String returns the NFC wire spelling.
func (p Path) String() string { return p.s }

// IsZero reports the absent-path sentinel.
func (p Path) IsZero() bool { return p.s == "" }

// Key returns the versioned filesystem uniqueness key.
func (p Path) Key() Key { return pathKey(p.s) }
func pathKey(s string) Key {
	return Key(NFC(cases.Fold().String(norm.NFD.String(strings.Map(unicode.ToUpper, s)))))
}

// KeyScheme pins algorithm and all Unicode tables, including Go's simple uppercase.
func KeyScheme() string {
	return "v1:upper=" + unicode.Version + ":fold=" + cases.UnicodeVersion + ":norm=" + norm.Version
}

// Base returns the final component, or empty for zero.
func (p Path) Base() string { i := strings.LastIndexByte(p.s, '/'); return p.s[i+1:] }

// Dir returns the parent; false means the path is at the vault root.
func (p Path) Dir() (Path, bool) {
	i := strings.LastIndexByte(p.s, '/')
	if i < 0 {
		return Path{}, false
	}
	return Path{p.s[:i]}, true
}

// Ext returns the lowercase final extension, excluding its dot and leading dots.
func (p Path) Ext() string {
	b := p.Base()
	i := strings.LastIndexByte(b, '.')
	if i <= 0 {
		return ""
	}
	return strings.ToLower(b[i+1:])
}

// Join appends exactly one validated component. Zero denotes the vault root.
func (p Path) Join(name string) (Path, error) {
	n, e := Parse(name)
	if e != nil {
		return Path{}, e
	}
	if strings.ContainsRune(n.s, '/') {
		return Path{}, &PathError{name, 0, IllegalChar}
	}
	if p.IsZero() {
		return n, nil
	}
	return Parse(p.s + "/" + name)
}

// WithBase replaces exactly one final component, retaining the directory.
func (p Path) WithBase(name string) (Path, error) { d, _ := p.Dir(); return d.Join(name) }

// Within tests component containment by key, including equality; zero never matches.
func (p Path) Within(dir Path) bool {
	if p.IsZero() || dir.IsZero() {
		return false
	}
	k, d := string(p.Key()), string(dir.Key())
	return k == d || strings.HasPrefix(k, d+"/")
}
