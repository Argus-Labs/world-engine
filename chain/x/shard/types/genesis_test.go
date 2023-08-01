package types

import (
	"gotest.tools/v3/assert"
	"testing"
)

func TestGenesisValidate(t *testing.T) {
	testCases := []struct {
		name   string
		mutate func(*GenesisState)
		err    string
	}{
		{
			name:   "empty state is ok",
			mutate: func(state *GenesisState) {},
		},
		{
			name: "no namespace",
			mutate: func(state *GenesisState) {
				state.NamespaceTransactions = append(state.NamespaceTransactions, &NamespaceTransactions{})
			},
			err: "empty namespace",
		},
		{
			name: "no transactions",
			mutate: func(state *GenesisState) {
				state.NamespaceTransactions[0].Namespace = "foo"
			},
			err: "no transactions for namespace foo",
		},
		{
			name: "no transactions for tick",
			mutate: func(state *GenesisState) {
				state.NamespaceTransactions[0].Ticks = append(state.NamespaceTransactions[0].Ticks, &Tick{})
			},
			err: "no transactions for tick 0 in namespace foo",
		},
		{
			name: "empty signed payload",
			mutate: func(state *GenesisState) {
				state.NamespaceTransactions[0].Ticks[0].Txs = &Transactions{Txs: []*Transaction{{}}}
			},
			err: "no transaction data",
		},
	}
	g := &GenesisState{}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mutate(g)
			err := g.Validate()
			if tc.err != "" {
				assert.ErrorContains(t, err, tc.err)
			} else {
				assert.NilError(t, err)
			}
		})
	}
}
