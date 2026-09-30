package phasebox

// RowState is the lifecycle state of a single row within a phasebox.
type RowState int

const (
	// Pending marks a row that hasn't started yet (e.g. a build queued
	// behind another).
	Pending RowState = iota
	// Active marks a row currently in progress; it renders with the box's
	// shared animated spinner frame.
	Active
	// Done marks a row that finished successfully.
	Done
	// Failed marks a row that errored.
	Failed
)

// Session is the live handle into a running phasebox, scoped to the
// lifetime of one Run call. UpsertRow/UpsertProgress insert a row on first
// use (by id) and update it on every subsequent call; rows render in
// first-insertion order.
type Session interface {
	// UpsertRow renders id's row as an icon (per state) + label + detail.
	UpsertRow(id, label, detail string, state RowState)
	// UpsertProgress renders id's row as label + a percent-complete bar
	// (e.g. "golang:1.26.2 ████████░░░░░░░░ 42%") instead of an icon, for
	// determinate operations like image pulls. Always implicitly Active;
	// follow with UpsertRow(..., Done/Failed) to swap back to an icon row.
	UpsertProgress(id, label string, percent int)
}
