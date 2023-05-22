package router

import "cosmossdk.io/core/appmodule"

func init() {
	appmodule.Register(&modulev1alpha1.Module{}, appmodule.Provide(ProvideModule))
}
