package phasebox

import (
	"strings"

	"github.com/argus-labs/world-engine/cli/pkg/docker"
)

// maxDetailLen bounds how much of a k3d log line a row shows, so a long one
// can't blow out the box's width.
const maxDetailLen = 96

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

// ClusterLogRow returns an OnK3DLog-compatible func(string) that
// live-updates one row's detail with each k3d log line. Each call site owns
// its own Session, so — unlike the bare-spinner approach it replaces — no
// global routing between concurrent sessions is needed.
func ClusterLogRow(sess Session, id, label string) func(line string) {
	return func(line string) {
		sess.UpsertRow(id, label, truncate(line), Active)
	}
}

// truncate collapses newlines and caps s to maxDetailLen runes so a long k3d
// log line can't blow out the box's width.
func truncate(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) <= maxDetailLen {
		return s
	}
	return string(r[:maxDetailLen-1]) + "…"
}
