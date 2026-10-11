package rules

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/BarneyLaw/stele-sync/internal/core/classify"
	"github.com/BarneyLaw/stele-sync/internal/core/vpath"
)

// Actor identifies the authority of a submission.
type Actor uint8

const (
	// Device is a user device.
	Device Actor = iota
	// Bridge is the Canvas importer.
	Bridge
	// Admin is the trusted system actor.
	Admin
)

// Owner is the immutable ownership assigned at creation.
type Owner uint8

const (
	// User permits device edits.
	User Owner = iota
	// Canvas permits bridge edits.
	Canvas
)

// Holder reports who holds a target key in the locked namespace snapshot.
type Holder uint8

const (
	// Free means no live holder.
	Free Holder = iota
	// Self permits case-only renames.
	Self
	// Other means a different live file.
	Other
)

// OnTaken selects automatic naming or rejection.
type OnTaken uint8

const (
	// Number requests a server-assigned numbered name.
	Number OnTaken = iota
	// Reject requests PathTaken on collision.
	Reject
)

// File is an immutable snapshot; its class and ownership never change in place.
type File struct {
	Live  bool
	Head  int64
	Class classify.Class
	Owner Owner
	Path  vpath.Path
	Size  int64
}

// Decision is mapped to wire codes once, in session.
type Decision uint8

const (
	// Apply accepts the requested operation.
	Apply Decision = iota + 1
	// ApplyNumbered requires the shell to reserve a server-assigned name.
	ApplyNumbered
	// PathTaken rejects a held target key.
	PathTaken
	// StaleDelete preserves a newer head.
	StaleDelete
	// FileDeleted reports a tombstoned file.
	FileDeleted
	// ReadOnlyPath rejects an ownership violation.
	ReadOnlyPath
	// ClassChange requires a new file identity.
	ClassChange
	// ConflictCopy archives stale content under a server-assigned name.
	ConflictCopy
	// RebaseRequired rejects unsafe reconciliation.
	RebaseRequired
	// NeedsConfirm pauses unconfirmed bulk deletion.
	NeedsConfirm
	// InvalidBase rejects a negative or future version.
	InvalidBase
	// GroupRequired requires a valid PDF/sidecar rename group.
	GroupRequired
)

// Config is validated by the shell once. Classify must match creation classification.
type Config struct {
	CanvasRoot  vpath.Path
	Compound    []string
	MaxAttempts int
	Quiet       time.Duration
	Window      Window
	Guard       GuardConfig
	Classify    classify.Config
}

// DefaultConfig supplies the M3 naming, retention and deletion defaults.
func DefaultConfig() Config {
	root, _ := vpath.Parse("Canvas")
	return Config{CanvasRoot: root, Compound: []string{".annot.json"}, MaxAttempts: 100, Quiet: 10 * time.Second, Window: Window{5000, 24 * time.Hour}, Guard: GuardConfig{5 * time.Minute, 25, 5, 0.10, 30}, Classify: classify.DefaultConfig()}
}

var (
	// ErrNoFreeName reports exhausted bounded name allocation.
	ErrNoFreeName = errors.New("rules: no free name")
	// ErrNameTooLong reports a candidate or PDF pair that cannot fit.
	ErrNameTooLong = errors.New("rules: name too long")
	// ErrInvalidConfig reports unusable naming or guard configuration.
	ErrInvalidConfig = errors.New("rules: invalid config")
	// ErrNotPDF reports a non-PDF input to SidecarOf.
	ErrNotPDF = errors.New("rules: not a PDF")
)

// Validate reports all configuration problems; no partial configuration is used.
func (c Config) Validate() error {
	var errs []error
	if c.CanvasRoot.IsZero() || c.MaxAttempts <= 0 || c.MaxAttempts > 10000 || c.Quiet < 0 || c.Window.TextMaxOps < 0 || c.Window.TextMaxAge < 0 {
		errs = append(errs, ErrInvalidConfig)
	}
	seen := map[string]bool{}
	for _, s := range c.Compound {
		lower := strings.ToLower(s)
		_, e := vpath.Parse("x" + s)
		if len(s) < 2 || s[0] != '.' || len(s) > 64 || strings.ContainsAny(s, "/\\") || e != nil || seen[lower] {
			errs = append(errs, fmt.Errorf("%w: compound %q", ErrInvalidConfig, s))
		}
		seen[lower] = true
	}
	if e := c.Classify.Validate(); e != nil {
		errs = append(errs, fmt.Errorf("%w: %w", ErrInvalidConfig, e))
	}
	if e := c.Guard.Validate(); e != nil {
		errs = append(errs, e)
	}
	return errors.Join(errs...)
}
