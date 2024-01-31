// Package router provides functionality for Cardinal to interact with the EVM Base Shard.
// This involves a few responsibilities:
//   - Receiving API requests from EVM smart contracts on the base shard.
//   - Sending transactions to the base shard's game sequencer.
//   - Querying transactions from the base shard to rebuild game state.
package router
