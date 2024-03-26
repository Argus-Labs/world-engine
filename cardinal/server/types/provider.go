package types

import (
	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/search"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/sign"
)

type Provider interface {
	GetSignerForPersonaTag(personaTag string, tick uint64) (addr string, err error)
	AddTransaction(id types.MessageID, v any, sig *sign.Transaction) (uint64, types.TxHash)
	Namespace() string
	GetComponentByName(name string) (types.ComponentMetadata, error)
	Search(filter filter.ComponentFilter) *search.Search
	StoreReader() gamestate.Reader
}
