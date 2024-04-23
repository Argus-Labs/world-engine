package main

import (
	"errors"
	"os"
	"runtime/pprof"
	"time"

	"github.com/rs/zerolog/log"

	"pkg.world.dev/world-engine/cardinal"

	"github.com/argus-labs/world-engine/example/tester/agarbenchmark/component"
	"github.com/argus-labs/world-engine/example/tester/agarbenchmark/msg"
	"github.com/argus-labs/world-engine/example/tester/agarbenchmark/query"
	"github.com/argus-labs/world-engine/example/tester/agarbenchmark/system"
)

func main() {

	// This code is a bit redundant will change.
	filename := "agar.cpu.prof"
	folder := "/profiles/"
	fullFilename := folder + filename
	profileFile, err := os.Create(fullFilename)
	if err != nil {
		log.Fatal()
	}
	defer profileFile.Close()

	if err := pprof.StartCPUProfile(profileFile); err != nil {
		panic("could not start CPU profile: " + err.Error())
	}
	defer pprof.StopCPUProfile()

	duration, _ := time.ParseDuration(".05s")
	w, err := cardinal.NewWorld(cardinal.WithDisableSignatureVerification(), cardinal.WithTickChannel(time.Tick(duration)))
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	EnterWorld(w)

	Must(w.StartGame())
}

func EnterWorld(w *cardinal.World) {
	// Register components
	// NOTE: You must register your components here for it to be accessible.
	Must(
		cardinal.RegisterComponent[component.Bearing](w),
		cardinal.RegisterComponent[component.Coin](w),
		cardinal.RegisterComponent[component.Health](w),
		cardinal.RegisterComponent[component.LastObservedTick](w),
		cardinal.RegisterComponent[component.Level](w),
		cardinal.RegisterComponent[component.LinearVelocity](w),
		cardinal.RegisterComponent[component.Medpack](w),
		cardinal.RegisterComponent[component.Offense](w),
		cardinal.RegisterComponent[component.Pickup](w),
		cardinal.RegisterComponent[component.Player](w),
		cardinal.RegisterComponent[component.Position](w),
		cardinal.RegisterComponent[component.Radius](w),
		cardinal.RegisterComponent[component.Reloader](w),
		cardinal.RegisterComponent[component.RigidBody](w),
		cardinal.RegisterComponent[component.Score](w),
		cardinal.RegisterComponent[component.Wealth](w),
	)

	// Register messages (user action)
	// NOTE: You must register your transactions here for it to be executed.
	Must(
		cardinal.RegisterMessage[msg.ChangeBearingMsg, msg.ChangeBearingResult](w, "change-bearing"),
		cardinal.RegisterMessage[msg.ChangeLinearVelocityMsg, msg.ChangeLinearVelocityResult](w, "change-linear-velocity"),
		cardinal.RegisterMessage[msg.CreatePlayerMsg, msg.CreatePlayerResult](w, "create-player"),
		cardinal.RegisterMessage[msg.KeepAliveMsg, msg.KeepAliveResult](w, "keep-alive"),
	)

	// Register queries
	// NOTE: You must register your queries here for it to be accessible.
	Must(
		cardinal.RegisterQuery(w, "world-vars", query.WorldVars),
		cardinal.RegisterQuery(w, "world-state", query.WorldState),
	)

	// Each system executes deterministically in the order they are added.
	// This is a neat feature that can be strategically used for systems that depends on the order of execution.
	// For example, you may want to run the attack system before the regen system
	// so that the player's HP is subtracted (and player killed if it reaches 0) before HP is regenerated.
	Must(cardinal.RegisterSystems(w,
		system.PickupRecoveryInitSystem,
		system.PlayerRecoveryInitSystem,
		system.GameSystem,
		system.PlayerSpawnerSystem,
		system.CoinSpawnerSystem,
		system.MedpackSpawnerSystem,
		system.AttackSystem,
		system.PhysicsSystem,
		system.KeepAliveSystem,
		system.LateDestroyBodyAndThenRemoveEntitySystem,
		// system.TestEventsSystem, // Uncomment this line to stress test the event system and potentially break Nakama.
	))
}

func Must(err ...error) {
	e := errors.Join(err...)
	if e != nil {
		log.Fatal().Err(e).Msg("")
	}
}
