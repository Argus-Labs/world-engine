// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package precompile

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
	_ = abi.ConvertType
)

// DarkForestMsgSendEnergy is an auto generated low-level Go binding around an user-defined struct.
type DarkForestMsgSendEnergy struct {
	From   uint64
	To     uint64
	Amount uint64
}

// DarkForestMsgSendEnergyResponse is an auto generated low-level Go binding around an user-defined struct.
type DarkForestMsgSendEnergyResponse struct {
	Code    uint64
	Message string
}

// DarkForestMetaData contains all meta data concerning the DarkForest contract.
var DarkForestMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"components\":[{\"internalType\":\"uint64\",\"name\":\"From\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"To\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"Amount\",\"type\":\"uint64\"}],\"internalType\":\"structDarkForest.MsgSendEnergy\",\"name\":\"msg\",\"type\":\"tuple\"}],\"name\":\"SendEnergy\",\"outputs\":[{\"components\":[{\"internalType\":\"uint64\",\"name\":\"Code\",\"type\":\"uint64\"},{\"internalType\":\"string\",\"name\":\"Message\",\"type\":\"string\"}],\"internalType\":\"structDarkForest.MsgSendEnergyResponse\",\"name\":\"response\",\"type\":\"tuple\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
}

// DarkForestABI is the input ABI used to generate the binding from.
// Deprecated: Use DarkForestMetaData.ABI instead.
var DarkForestABI = DarkForestMetaData.ABI

// DarkForest is an auto generated Go binding around an Ethereum contract.
type DarkForest struct {
	DarkForestCaller     // Read-only binding to the contract
	DarkForestTransactor // Write-only binding to the contract
	DarkForestFilterer   // Log filterer for contract events
}

// DarkForestCaller is an auto generated read-only Go binding around an Ethereum contract.
type DarkForestCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// DarkForestTransactor is an auto generated write-only Go binding around an Ethereum contract.
type DarkForestTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// DarkForestFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type DarkForestFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// DarkForestSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type DarkForestSession struct {
	Contract     *DarkForest       // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// DarkForestCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type DarkForestCallerSession struct {
	Contract *DarkForestCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts     // Call options to use throughout this session
}

// DarkForestTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type DarkForestTransactorSession struct {
	Contract     *DarkForestTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts     // Transaction auth options to use throughout this session
}

// DarkForestRaw is an auto generated low-level Go binding around an Ethereum contract.
type DarkForestRaw struct {
	Contract *DarkForest // Generic contract binding to access the raw methods on
}

// DarkForestCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type DarkForestCallerRaw struct {
	Contract *DarkForestCaller // Generic read-only contract binding to access the raw methods on
}

// DarkForestTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type DarkForestTransactorRaw struct {
	Contract *DarkForestTransactor // Generic write-only contract binding to access the raw methods on
}

// NewDarkForest creates a new instance of DarkForest, bound to a specific deployed contract.
func NewDarkForest(address common.Address, backend bind.ContractBackend) (*DarkForest, error) {
	contract, err := bindDarkForest(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &DarkForest{DarkForestCaller: DarkForestCaller{contract: contract}, DarkForestTransactor: DarkForestTransactor{contract: contract}, DarkForestFilterer: DarkForestFilterer{contract: contract}}, nil
}

// NewDarkForestCaller creates a new read-only instance of DarkForest, bound to a specific deployed contract.
func NewDarkForestCaller(address common.Address, caller bind.ContractCaller) (*DarkForestCaller, error) {
	contract, err := bindDarkForest(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &DarkForestCaller{contract: contract}, nil
}

// NewDarkForestTransactor creates a new write-only instance of DarkForest, bound to a specific deployed contract.
func NewDarkForestTransactor(address common.Address, transactor bind.ContractTransactor) (*DarkForestTransactor, error) {
	contract, err := bindDarkForest(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &DarkForestTransactor{contract: contract}, nil
}

// NewDarkForestFilterer creates a new log filterer instance of DarkForest, bound to a specific deployed contract.
func NewDarkForestFilterer(address common.Address, filterer bind.ContractFilterer) (*DarkForestFilterer, error) {
	contract, err := bindDarkForest(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &DarkForestFilterer{contract: contract}, nil
}

// bindDarkForest binds a generic wrapper to an already deployed contract.
func bindDarkForest(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := DarkForestMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_DarkForest *DarkForestRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _DarkForest.Contract.DarkForestCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_DarkForest *DarkForestRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _DarkForest.Contract.DarkForestTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_DarkForest *DarkForestRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _DarkForest.Contract.DarkForestTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_DarkForest *DarkForestCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _DarkForest.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_DarkForest *DarkForestTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _DarkForest.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_DarkForest *DarkForestTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _DarkForest.Contract.contract.Transact(opts, method, params...)
}

// SendEnergy is a paid mutator transaction binding the contract method 0x6117e1c4.
//
// Solidity: function SendEnergy((uint64,uint64,uint64) msg) returns((uint64,string) response)
func (_DarkForest *DarkForestTransactor) SendEnergy(opts *bind.TransactOpts, msg DarkForestMsgSendEnergy) (*types.Transaction, error) {
	return _DarkForest.contract.Transact(opts, "SendEnergy", msg)
}

// SendEnergy is a paid mutator transaction binding the contract method 0x6117e1c4.
//
// Solidity: function SendEnergy((uint64,uint64,uint64) msg) returns((uint64,string) response)
func (_DarkForest *DarkForestSession) SendEnergy(msg DarkForestMsgSendEnergy) (*types.Transaction, error) {
	return _DarkForest.Contract.SendEnergy(&_DarkForest.TransactOpts, msg)
}

// SendEnergy is a paid mutator transaction binding the contract method 0x6117e1c4.
//
// Solidity: function SendEnergy((uint64,uint64,uint64) msg) returns((uint64,string) response)
func (_DarkForest *DarkForestTransactorSession) SendEnergy(msg DarkForestMsgSendEnergy) (*types.Transaction, error) {
	return _DarkForest.Contract.SendEnergy(&_DarkForest.TransactOpts, msg)
}
