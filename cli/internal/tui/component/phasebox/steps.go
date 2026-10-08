package phasebox

import "strconv"

// StepTracker turns a sequence of named steps (e.g. cluster.StartOpts.OnStep)
// into a live checklist of Session rows: each Next call marks the previous
// step Done and starts the next Active, showing which phase a multi-phase
// operation is in instead of one static spinner.
type StepTracker struct {
	sess  Session
	id    string
	label string
	n     int
}

// NewStepTracker returns a tracker that pushes rows into sess.
func NewStepTracker(sess Session) *StepTracker {
	return &StepTracker{sess: sess}
}

// Next marks the current step (if any) Done and starts a new Active row for
// label.
func (t *StepTracker) Next(label string) {
	if t.id != "" {
		t.sess.UpsertRow(t.id, t.label, "", Done)
	}
	t.n++
	t.id = strconv.Itoa(t.n)
	t.label = label
	t.sess.UpsertRow(t.id, label, "", Active)
}

// Detail updates the current step's trailing detail text (e.g. a log line)
// without changing its state. No-op before the first Next call.
func (t *StepTracker) Detail(detail string) {
	if t.id == "" {
		return
	}
	t.sess.UpsertRow(t.id, t.label, detail, Active)
}

// Done marks the current (final) step Done. Call once the whole sequence
// finishes successfully. No-op before the first Next call.
func (t *StepTracker) Done() {
	if t.id == "" {
		return
	}
	t.sess.UpsertRow(t.id, t.label, "", Done)
}

// Failed marks the current step failed (see Session.Fail). No-op before the
// first Next call.
func (t *StepTracker) Failed(err error) {
	if t.id == "" {
		return
	}
	t.sess.Fail(t.id, t.label, err)
}
