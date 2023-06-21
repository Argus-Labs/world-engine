package types

func DefaultGenesis() *Genesis {
	return &Genesis{}
}

func (g *Genesis) Validate() error {
	for _, ns := range g.Namespaces {
		if err := ns.Validate(); err != nil {
			return err
		}
	}
	return nil
}
