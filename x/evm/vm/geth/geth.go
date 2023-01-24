package geth

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"

	evm "github.com/argus-labs/argus/x/evm/vm"
)

var (
	_ evm.EVM         = (*EVM)(nil)
	_ evm.Constructor = NewEVM
)

// EVM is the wrapper for the go-ethereum EVM.
type EVM struct {
	*vm.EVM
	*evm.ContractAllowlistOption
}

// NewEVM defines the constructor function for the go-ethereum (geth) EVM. It uses
// the default precompiled contracts and the EVM concrete implementation from
// geth.
func NewEVM(
	blockCtx vm.BlockContext,
	txCtx vm.TxContext,
	stateDB vm.StateDB,
	chainConfig *params.ChainConfig,
	config vm.Config,
	_ evm.PrecompiledContracts, // unused
	allowlistOpt *evm.ContractAllowlistOption,
) evm.EVM {
	return &EVM{
		EVM:                     vm.NewEVM(blockCtx, txCtx, stateDB, chainConfig, config),
		ContractAllowlistOption: allowlistOpt,
	}
}

// Context returns the EVM's Block Context
func (e EVM) Context() vm.BlockContext {
	return e.EVM.Context
}

// TxContext returns the EVM's Tx Context
func (e EVM) TxContext() vm.TxContext {
	return e.EVM.TxContext
}

// Config returns the configuration options for the EVM.
func (e EVM) Config() vm.Config {
	return e.EVM.Config
}

// Precompile returns the precompiled contract associated with the given address
// and the current chain configuration. If the contract cannot be found it returns
// nil.
func (e EVM) Precompile(addr common.Address) (p vm.PrecompiledContract, found bool) {
	precompiles := GetPrecompiles(e.ChainConfig(), e.EVM.Context.BlockNumber)
	p, found = precompiles[addr]
	return p, found
}

// ActivePrecompiles returns a list of all the active precompiled contract addresses
// for the current chain configuration.
func (EVM) ActivePrecompiles(rules params.Rules) []common.Address {
	return vm.ActivePrecompiles(rules)
}

// RunPrecompiledContract runs a stateless precompiled contract and ignores the address and
// value arguments. It uses the RunPrecompiledContract function from the geth vm package.
func (EVM) RunPrecompiledContract(
	p evm.StatefulPrecompiledContract,
	_ common.Address, // address arg is unused
	input []byte,
	suppliedGas uint64,
	_ *big.Int, // 	value arg is unused
) (ret []byte, remainingGas uint64, err error) {
	return vm.RunPrecompiledContract(p, input, suppliedGas)
}

// Create creates a new contract using code as deployment code.
func (e *EVM) Create(caller vm.ContractRef, code []byte, gas uint64, value *big.Int) (ret []byte, contractAddr common.Address, leftOverGas uint64, err error) {
	if err := e.BeforeCreate(caller.Address().String()); err != nil {
		return nil, caller.Address(), gas, err
	}
	return e.EVM.Create(caller, code, gas, value)
}

// Create2 creates a new contract using code as deployment code.
//
// The different between Create2 with Create is Create2 uses keccak256(0xff ++ msg.sender ++ salt ++ keccak256(init_code))[12:]
// instead of the usual sender-and-nonce-hash as the address where the contract is initialized at.
func (e *EVM) Create2(caller vm.ContractRef, code []byte, gas uint64, endowment *big.Int, salt *uint256.Int) (ret []byte, contractAddr common.Address, leftOverGas uint64, err error) {
	if err := e.BeforeCreate(caller.Address().String()); err != nil {
		return nil, caller.Address(), gas, err
	}
	return e.EVM.Create2(caller, code, gas, endowment, salt)
}

// BeforeCreate runs any logic needed before contract creation begins.
func (e *EVM) BeforeCreate(caller string) error {
	// first check if the option is enabled
	if e.ContractAllowlistOption != nil {
		// check the allowlist contains the caller
		if !e.CanCreate(caller) {
			return fmt.Errorf("%s is not allowed to create contracts", caller)
		}
	}
	return nil
}
