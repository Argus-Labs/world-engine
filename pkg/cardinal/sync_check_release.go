//go:build release

package cardinal

// check is compiled out of release builds; see sync_check.go.
func (p *syncPublisher) check() {}
