// Package ids defines canonical UUIDv7 identities and JavaScript-safe counters.
// Generation takes explicit time and entropy; identifiers never read a clock.
package ids

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// MaxSafe is the largest exactly representable JavaScript integer.
const MaxSafe = 1<<53 - 1

var (
	// ErrInvalidID indicates a noncanonical UUIDv7 or invalid generation input.
	ErrInvalidID = errors.New("ids: invalid UUIDv7")
	// ErrUnsafeInt indicates a counter outside its interoperable range.
	ErrUnsafeInt = errors.New("ids: unsafe integer")
)

// Version numbers committed file states starting at one.
type Version int64

// Seq numbers vault transactions; zero precedes the first transaction.
type Seq int64

// Valid reports whether the version can identify a committed file state.
func (v Version) Valid() bool { return v >= 1 && v <= MaxSafe }

// Valid reports whether the cursor is interoperable with JavaScript.
func (s Seq) Valid() bool { return s >= 0 && s <= MaxSafe }

func parse(s string) ([16]byte, error) {
	var out [16]byte
	if len(s) != 36 {
		return out, ErrInvalidID
	}
	var compact [32]byte
	j := 0
	for i := range len(s) {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if s[i] != '-' {
				return out, ErrInvalidID
			}
			continue
		}
		if (s[i] < '0' || s[i] > '9') && (s[i] < 'a' || s[i] > 'f') {
			return out, ErrInvalidID
		}
		compact[j] = s[i]
		j++
	}
	if _, err := hex.Decode(out[:], compact[:]); err != nil {
		return [16]byte{}, ErrInvalidID
	}
	if !valid(out) {
		return [16]byte{}, ErrInvalidID
	}
	return out, nil
}
func valid(id [16]byte) bool { return id[6]>>4 == 7 && id[8]>>6 == 2 }
func generate(ms int64, entropy io.Reader) ([16]byte, error) {
	var id [16]byte
	if ms < 0 || ms >= 1<<48 || entropy == nil {
		return id, ErrInvalidID
	}
	if _, err := io.ReadFull(entropy, id[6:]); err != nil {
		return id, fmt.Errorf("ids: entropy: %w", err)
	}
	for i := 5; i >= 0; i-- {
		id[i] = byte(ms & 255)
		ms >>= 8
	}
	id[6] = id[6]&15 | 0x70
	id[8] = id[8]&63 | 0x80
	return id, nil
}
func format(id [16]byte) string {
	var out [36]byte
	hex.Encode(out[:8], id[:4])
	out[8] = '-'
	hex.Encode(out[9:13], id[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], id[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], id[8:10])
	out[23] = '-'
	hex.Encode(out[24:], id[10:])
	return string(out[:])
}

// FileID identifies a file independently of its path.
type FileID [16]byte

// ParseFileID accepts only lowercase canonical UUIDv7 with the RFC 9562 variant.
func ParseFileID(s string) (FileID, error) { v, e := parse(s); return FileID(v), e }

// NewFileID constructs a UUIDv7 from a 48-bit Unix millisecond time and ten entropy bytes.
func NewFileID(unixMilli int64, entropy io.Reader) (FileID, error) {
	v, e := generate(unixMilli, entropy)
	return FileID(v), e
}

// String returns the canonical identifier spelling.
func (id FileID) String() string { return format([16]byte(id)) }

// MarshalText rejects invalid values, including the zero identifier.
func (id FileID) MarshalText() ([]byte, error) {
	if !valid([16]byte(id)) {
		return nil, ErrInvalidID
	}
	return []byte(id.String()), nil
}

// UnmarshalText replaces the receiver only when the input is valid.
func (id *FileID) UnmarshalText(b []byte) error {
	v, e := ParseFileID(string(b))
	if e == nil {
		*id = v
	}
	return e
}

// DeviceID identifies an enrolled or system device.
type DeviceID [16]byte

// ParseDeviceID accepts only lowercase canonical UUIDv7 with the RFC 9562 variant.
func ParseDeviceID(s string) (DeviceID, error) { v, e := parse(s); return DeviceID(v), e }

// NewDeviceID constructs a UUIDv7 from a 48-bit Unix millisecond time and ten entropy bytes.
func NewDeviceID(unixMilli int64, entropy io.Reader) (DeviceID, error) {
	v, e := generate(unixMilli, entropy)
	return DeviceID(v), e
}

// String returns the canonical identifier spelling.
func (id DeviceID) String() string { return format([16]byte(id)) }

// MarshalText rejects invalid values, including the zero identifier.
func (id DeviceID) MarshalText() ([]byte, error) {
	if !valid([16]byte(id)) {
		return nil, ErrInvalidID
	}
	return []byte(id.String()), nil
}

// UnmarshalText replaces the receiver only when the input is valid.
func (id *DeviceID) UnmarshalText(b []byte) error {
	v, e := ParseDeviceID(string(b))
	if e == nil {
		*id = v
	}
	return e
}

// ClientOpID identifies a submission across retries and catch-up.
type ClientOpID [16]byte

// ParseClientOpID accepts only lowercase canonical UUIDv7 with the RFC 9562 variant.
func ParseClientOpID(s string) (ClientOpID, error) { v, e := parse(s); return ClientOpID(v), e }

// NewClientOpID constructs a UUIDv7 from a 48-bit Unix millisecond time and ten entropy bytes.
func NewClientOpID(unixMilli int64, entropy io.Reader) (ClientOpID, error) {
	v, e := generate(unixMilli, entropy)
	return ClientOpID(v), e
}

// String returns the canonical identifier spelling.
func (id ClientOpID) String() string { return format([16]byte(id)) }

// MarshalText rejects invalid values, including the zero identifier.
func (id ClientOpID) MarshalText() ([]byte, error) {
	if !valid([16]byte(id)) {
		return nil, ErrInvalidID
	}
	return []byte(id.String()), nil
}

// UnmarshalText replaces the receiver only when the input is valid.
func (id *ClientOpID) UnmarshalText(b []byte) error {
	v, e := ParseClientOpID(string(b))
	if e == nil {
		*id = v
	}
	return e
}
