package ecs

import "pkg.world.dev/world-engine/cardinal"

type System func(cardinal.WorldContext) error
