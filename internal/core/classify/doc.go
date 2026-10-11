// Package classify decides file classes from names, sizes and complete content.
// It is pure and never upgrades an expected opaque class. Annotation recognition
// is delegated to the pinned M2 parser; exclusion policy and blob limits belong
// to their own components. Callers validate Config once at their boundary.
package classify
