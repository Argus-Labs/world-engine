//go:build !release

package cardinal

import (
	"github.com/argus-labs/world-engine/pkg/assert"
)

// check asserts, at the end of every tick in development builds, that the publisher's derived
// state equals what it is derived from: every seated reader's slot bits equal the list test, and
// each policy lists exactly the entities whose entries use it.
func (p *syncPublisher) check() {
	indexed := 0
	for key, pol := range p.policies {
		assert.That(len(pol.entities) > 0 && !pol.stale && pol.key == key,
			"sync policy %q kept with %d entities, stale=%v", key, len(pol.entities), pol.stale)
		indexed += len(pol.entities)
		for _, n := range pol.entities {
			assert.That(p.entry(n) != nil && p.entry(n).policy == pol,
				"sync policy %q lists entity %d, which does not use it", key, n)
		}
		p.forSeated(func(slot int, player string) {
			assert.That(
				pol.view.has(slot) == pol.lists[0].has(player) && pol.auth.has(slot) == pol.lists[1].has(player),
				"sync policy %q has the wrong bits for %s in slot %d",
				key,
				player,
				slot,
			)
		})
	}
	for name, g := range p.groups {
		p.forSeated(func(slot int, player string) {
			_, member := g.members[player]
			assert.That(
				g.mask.has(slot) == member,
				"sync group %q has the wrong bit for %s in slot %d",
				name,
				player,
				slot,
			)
		})
	}
	published := 0
	for _, e := range p.entries {
		if e != nil {
			published++
		}
	}
	assert.That(indexed == published, "sync policies list %d entities, %d are published", indexed, published)
	for slot, r := range p.roster.readers {
		assert.That(
			(r.player != "") == p.roster.live.has(slot) && (r.player == "" || p.roster.slotOf[r.player] == slot),
			"sync slot %d holds %q, open=%v",
			slot,
			r.player,
			p.roster.live.has(slot),
		)
	}
}

func (p *syncPublisher) forSeated(fn func(slot int, player string)) {
	for slot, r := range p.roster.readers {
		if r.player != "" {
			fn(slot, r.player)
		}
	}
}
