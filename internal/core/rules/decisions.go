package rules

import (
	"time"

	"github.com/BarneyLaw/stele-sync/internal/core/classify"
	"github.com/BarneyLaw/stele-sync/internal/core/vpath"
)

func allowed(a Actor, f File, root vpath.Path) bool {
	switch a {
	case Admin:
		return true
	case Device:
		return f.Owner == User && (f.Class != classify.Opaque || !f.Path.Within(root))
	case Bridge:
		return f.Owner == Canvas && f.Class == classify.Opaque && f.Path.Within(root)
	}
	return false
}
func destinationAllowed(a Actor, p vpath.Path, c classify.Class, root vpath.Path) bool {
	owner := User
	if a == Bridge {
		owner = Canvas
	}
	return allowed(a, File{Class: c, Owner: owner, Path: p}, root)
}
func collision(h Holder, on OnTaken, rename bool) Decision {
	if h == Free || rename && h == Self {
		return Apply
	}
	if on == Number {
		return ApplyNumbered
	}
	return PathTaken
}

// DecideCreate checks ownership before resolving the requested target key.
func DecideCreate(a Actor, to vpath.Path, c classify.Class, h Holder, on OnTaken, cfg Config) Decision {
	if !destinationAllowed(a, to, c, cfg.CanvasRoot) {
		return ReadOnlyPath
	}
	return collision(h, on, false)
}

// RenameRequest describes an atomic single-file or PDF/sidecar rename.
// SidecarHolder is required because a free PDF name does not imply a free sidecar.
type RenameRequest struct {
	Primary       File
	To            vpath.Path
	Holder        Holder
	Sidecar       *File
	SidecarTo     vpath.Path
	SidecarLive   bool
	OnTaken       OnTaken
	SidecarHolder Holder
}

// DecideRename checks every member before accepting the atomic group.
func DecideRename(a Actor, r RenameRequest, cfg Config) Decision {
	if !allowed(a, r.Primary, cfg.CanvasRoot) || !destinationAllowed(a, r.To, r.Primary.Class, cfg.CanvasRoot) {
		return ReadOnlyPath
	}
	if r.Sidecar != nil && (!allowed(a, *r.Sidecar, cfg.CanvasRoot) || !destinationAllowed(a, r.SidecarTo, r.Sidecar.Class, cfg.CanvasRoot)) {
		return ReadOnlyPath
	}
	if !r.Primary.Live {
		return FileDeleted
	}
	if isPDF(r.Primary.Path) && r.SidecarLive && r.Sidecar == nil {
		return GroupRequired
	}
	if r.Sidecar != nil {
		from, e := SidecarOf(r.Primary.Path)
		to, err := SidecarOf(r.To)
		if e != nil || err != nil || from.Key() != r.Sidecar.Path.Key() || to != r.SidecarTo {
			return GroupRequired
		}
		if !r.Sidecar.Live {
			return FileDeleted
		}
	}
	primary := renameMember(r.Primary, r.To, r.Holder, r.OnTaken, cfg)
	if primary != Apply && primary != ApplyNumbered {
		return primary
	}
	if r.Sidecar != nil {
		d := renameMember(*r.Sidecar, r.SidecarTo, r.SidecarHolder, r.OnTaken, cfg)
		if d != Apply && d != ApplyNumbered {
			return d
		}
		if d == ApplyNumbered {
			return d
		}
	}
	return primary
}
func renameMember(f File, to vpath.Path, h Holder, on OnTaken, cfg Config) Decision {
	if f.Path == to {
		return Apply
	}
	if classify.Expect(f.Path, f.Size, cfg.Classify) != classify.Expect(to, f.Size, cfg.Classify) {
		return ClassChange
	}
	return collision(h, on, true)
}

// DecideEdit accepts A/B operations; reconciliation is checked separately.
func DecideEdit(a Actor, f File, cfg Config) Decision {
	if !allowed(a, f, cfg.CanvasRoot) {
		return ReadOnlyPath
	}
	if !f.Live {
		return FileDeleted
	}
	if f.Class != classify.Text && f.Class != classify.Sidecar {
		return ClassChange
	}
	return Apply
}
func baseDecision(a Actor, f File, base int64, stale Decision, cfg Config) Decision {
	if !allowed(a, f, cfg.CanvasRoot) {
		return ReadOnlyPath
	}
	if !f.Live {
		return FileDeleted
	}
	if base < 0 || base > f.Head {
		return InvalidBase
	}
	if base < f.Head {
		return stale
	}
	return Apply
}

// DecidePut applies opaque content only at head, otherwise preserving a conflict copy.
func DecidePut(a Actor, f File, base int64, cfg Config) Decision {
	d := baseDecision(a, f, base, ConflictCopy, cfg)
	if d == ReadOnlyPath || d == FileDeleted {
		return d
	}
	if f.Class != classify.Opaque {
		return ClassChange
	}
	return d
}

// DecideReplace gates the trusted whole-document replacement required by M2.
func DecideReplace(a Actor, f File, base int64, cfg Config) Decision {
	d := baseDecision(a, f, base, ConflictCopy, cfg)
	if d == ReadOnlyPath || d == FileDeleted {
		return d
	}
	if f.Class != classify.Sidecar {
		return ClassChange
	}
	return d
}

// DecideDelete preserves newer heads. Apply must subsequently pass DeleteGuard.
func DecideDelete(a Actor, f File, base int64, cfg Config) Decision {
	return baseDecision(a, f, base, StaleDelete, cfg)
}

// Gap describes server-measured history since the base version.
type Gap struct {
	Ops                         int
	Age                         time.Duration
	OpsPruned, TombstonesPruned bool
}

// Window bounds text reconciliation by server commit age and operation count.
type Window struct {
	TextMaxOps int
	TextMaxAge time.Duration
}

// Reconcile applies class-specific retention rules. Equality is inside the window.
func Reconcile(c classify.Class, g Gap, w Window) Decision {
	switch c {
	case classify.Text:
		if g.OpsPruned || g.Ops > w.TextMaxOps || g.Age > w.TextMaxAge {
			return RebaseRequired
		}
	case classify.Sidecar:
		if g.TombstonesPruned {
			return RebaseRequired
		}
	case classify.Opaque:
		return Apply
	default:
		return RebaseRequired
	}
	return Apply
}
