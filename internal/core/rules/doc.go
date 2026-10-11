// Package rules makes pure namespace, ownership, staleness, naming, retention,
// settling and deletion decisions. The shell validates configuration, supplies
// time and locked namespace lookups, then commits groups atomically. It remains
// responsible for uniqueness constraints and mapping decisions to wire codes.
package rules
