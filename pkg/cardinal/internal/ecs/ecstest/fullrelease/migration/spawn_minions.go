package migration

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/component"
)

// SpawnMinions is case 3b: it brings entities into existence during a restore.
//
// Create returns each new entity's ID immediately, so the minion can record who owns it in the
// same pass. That is also why the order has to be fixed rather than merely valid: a new entity
// takes the next free ID, so two shards restoring the same save with the same build have to create
// them in the same sequence to end up with the same world.
//
// The gate on whole-world migrations matters most here. Without it this would run at every boot
// and spawn the minions again.
type SpawnMinions struct {
	Spawners ecs.All[component.Spawner]
	Made     ecs.Create[component.Minion]
	Write    ecs.Put[component.Spawner]

	// Calls counts how many times the whole-world pass ran this.
	Calls int
	// Created counts the entities made, so a test can tell one spawner of three from three of one.
	Created int
}

func (m *SpawnMinions) Migrate() {
	m.Calls++

	for id, spawner := range m.Spawners.Iter() {
		for range spawner.Count {
			m.Made.New(component.Minion{Owner: uint32(id)})
			m.Created++
		}
		spawner.Spawned = spawner.Count
		m.Write.Set(id, spawner)
	}
}
