package types

func DefaultGenesis() *GenesisState {
	return &GenesisState{}
}

func (g *GenesisState) Validate() error {
	for _, txb := range g.Batches {
		if err := txb.Validate(); err != nil {
			return err
		}
	}
	return nil
}
