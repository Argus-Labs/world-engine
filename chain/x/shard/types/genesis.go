package types

import "fmt"

func DefaultGenesis() *GenesisState {
	return &GenesisState{}
}

func (g *GenesisState) Validate() error {
	for i, nstx := range g.NamespaceTransactions {
		if nstx.Namespace == "" {
			return fmt.Errorf("empty namespace at %d", i)
		}
		if nstx.Ticks == nil {
			return fmt.Errorf("no transactions for namespace %s", nstx.Namespace)
		}
		for _, tickedTxs := range nstx.Ticks {
			if tickedTxs.Txs == nil {
				return fmt.Errorf("no transactions for tick %d in namespace %s", tickedTxs.Tick, nstx.Namespace)
			}
			for j, tx := range tickedTxs.Txs {
				if tx.SignedPayload == nil {
					return fmt.Errorf("no transaction data for tx %d in tick %d in namespace %s", j,
						tickedTxs.Tick, nstx.Namespace)
				}
			}
		}
	}
	return nil
}
