package app

import "github.com/cosmos/cosmos-sdk/x/auth/types"

var (
	RouterName          = "base_shard_router"
	RouterModuleAddress = types.NewModuleAddress(RouterName)
)
