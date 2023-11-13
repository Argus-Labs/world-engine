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
		if nstx.Epochs == nil {
			return fmt.Errorf("no transactions for namespace %s", nstx.Namespace)
		}
		for _, epochTxs := range nstx.Epochs {
			if epochTxs.Txs == nil {
				return fmt.Errorf("no transactions for epoch %d in namespace %s", epochTxs.Epoch, nstx.Namespace)
			}
			for j, tx := range epochTxs.Txs {
				if tx.GameShardTransaction == nil {
					return fmt.Errorf("no transaction data for tx %d in epoch %d in namespace %s", j,
						epochTxs.Epoch, nstx.Namespace)
				}
			}
		}
	}
	return nil
}
