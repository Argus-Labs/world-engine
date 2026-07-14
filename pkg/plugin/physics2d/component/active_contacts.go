package component

import "github.com/argus-labs/world-engine/pkg/cardinal"

// PhysicsSingletonTag marks the single entity that holds physics plugin state (ActiveContacts).
type PhysicsSingletonTag struct{}

func (PhysicsSingletonTag) Name() string { return "physics_singleton_tag" }

// ContactPairEntry is one active contact pair tracked by the physics engine. Entries are
// normalized: EntityA < EntityB (or if equal, ShapeIndexA <= ShapeIndexB).
type ContactPairEntry struct {
	EntityA     cardinal.EntityID `json:"a"`
	ShapeIndexA int               `json:"sa"`
	EntityB     cardinal.EntityID `json:"b"`
	ShapeIndexB int               `json:"sb"`
	IsSensor    bool              `json:"sensor"`
	// Fixture filters for normalized EntityA/B (recovery End / trigger vs contact routing).
	// Omitempty keeps older snapshots valid.
	FilterACategoryBits uint64 `json:"fa_cat,omitempty"`
	FilterAMaskBits     uint64 `json:"fa_mask,omitempty"`
	FilterAGroupIndex   int32  `json:"fa_grp,omitempty"`
	FilterBCategoryBits uint64 `json:"fb_cat,omitempty"`
	FilterBMaskBits     uint64 `json:"fb_mask,omitempty"`
	FilterBGroupIndex   int32  `json:"fb_grp,omitempty"`
}

// ActiveContacts persists which contact pairs have had Begin emitted (and not yet End).
// After a rebuild, the physics step diffs this against Box2D's live contact list to emit
// correct Begin/End events without duplicates or missed ends.
type ActiveContacts struct {
	Pairs []ContactPairEntry `json:"pairs"`
}

func (ActiveContacts) Name() string { return "active_contacts" }
