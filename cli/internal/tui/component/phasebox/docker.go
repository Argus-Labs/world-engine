package phasebox

import (
	"github.com/argus-labs/world-engine/cli/pkg/docker"
)

// PullProgress adapts docker.Client.PullImages' progress into row updates:
// a real percent bar while pulling (Current is already 0-100), swapping to
// a done/failed icon row once the pull finishes.
func PullProgress(sess Session) func(docker.Progress) {
	return func(p docker.Progress) {
		switch {
		case p.Err != nil:
			sess.Fail(p.Name, p.Name, p.Err)
		case p.State == docker.StatePulled:
			sess.UpsertRow(p.Name, p.Name, "", Done)
		default:
			sess.UpsertProgress(p.Name, p.Name, int(p.Current))
		}
	}
}

// BuildProgress adapts docker.Client.BuildCardinalImages' progress
// callback. imageNames pre-seeds every row as Pending ("queued") before its
// first Progress event, so not-yet-started builds show immediately instead
// of popping in late.
func BuildProgress(sess Session, imageNames []string) func(docker.Progress) {
	for _, name := range imageNames {
		sess.UpsertRow(name, name, "queued", Pending)
	}
	return func(p docker.Progress) {
		switch {
		case p.Err != nil:
			sess.Fail(p.Name, p.Name, p.Err)
		case p.State == docker.StateBuilt:
			sess.UpsertRow(p.Name, p.Name, "", Done)
		default:
			sess.UpsertRow(p.Name, p.Name, "building…", Active)
		}
	}
}
