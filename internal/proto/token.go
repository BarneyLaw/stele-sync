package proto

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
)

// Token holds a device credential. Only the explicit wire encoder reveals it.
type Token struct{ secret string }

// ParseToken checks the stele prefix and 43 base64url characters.
func ParseToken(s string) (Token, error) {
	if len(s) != 49 || s[:6] != "stele_" {
		return Token{}, errors.New("proto: bad token")
	}
	for _, r := range s[6:] {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return Token{}, errors.New("proto: bad token")
		}
	}
	return Token{secret: s}, nil
}

// Hash computes the credential lookup hash without exposing its bytes.
func (t Token) Hash() [32]byte { return sha256.Sum256([]byte(t.secret)) }

// String redacts the credential.
func (Token) String() string { return "stele_[redacted]" }

// GoString redacts Go-syntax formatting.
func (t Token) GoString() string { return t.String() }

// Format redacts every fmt verb, including formatting through enclosing values.
func (t Token) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, t.String()) }

// LogValue redacts structured logging.
func (t Token) LogValue() slog.Value { return slog.StringValue(t.String()) }

// MarshalJSON redacts generic JSON serialization. EncodeClient is the only wire path.
func (Token) MarshalJSON() ([]byte, error) { return []byte(`"stele_[redacted]"`), nil }
