// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package namespace

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

// NamespaceMetaData contains all meta data concerning the Namespace contract.
var NamespaceMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"string\",\"name\":\"namespace\",\"type\":\"string\"}],\"name\":\"addressForNamespace\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"namespace\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"gRPCAddress\",\"type\":\"string\"}],\"name\":\"register\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
}

// NamespaceABI is the input ABI used to generate the binding from.
// Deprecated: Use NamespaceMetaData.ABI instead.
var NamespaceABI = NamespaceMetaData.ABI

// Namespace is an auto generated Go binding around an Ethereum contract.
type Namespace struct {
	NamespaceCaller     // Read-only binding to the contract
	NamespaceTransactor // Write-only binding to the contract
	NamespaceFilterer   // Log filterer for contract events
}

// NamespaceCaller is an auto generated read-only Go binding around an Ethereum contract.
type NamespaceCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// NamespaceTransactor is an auto generated write-only Go binding around an Ethereum contract.
type NamespaceTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// NamespaceFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type NamespaceFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// NamespaceSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type NamespaceSession struct {
	Contract     *Namespace        // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// NamespaceCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type NamespaceCallerSession struct {
	Contract *NamespaceCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts    // Call options to use throughout this session
}

// NamespaceTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type NamespaceTransactorSession struct {
	Contract     *NamespaceTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts    // Transaction auth options to use throughout this session
}

// NamespaceRaw is an auto generated low-level Go binding around an Ethereum contract.
type NamespaceRaw struct {
	Contract *Namespace // Generic contract binding to access the raw methods on
}

// NamespaceCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type NamespaceCallerRaw struct {
	Contract *NamespaceCaller // Generic read-only contract binding to access the raw methods on
}

// NamespaceTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type NamespaceTransactorRaw struct {
	Contract *NamespaceTransactor // Generic write-only contract binding to access the raw methods on
}

// NewNamespace creates a new instance of Namespace, bound to a specific deployed contract.
func NewNamespace(address common.Address, backend bind.ContractBackend) (*Namespace, error) {
	contract, err := bindNamespace(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Namespace{NamespaceCaller: NamespaceCaller{contract: contract}, NamespaceTransactor: NamespaceTransactor{contract: contract}, NamespaceFilterer: NamespaceFilterer{contract: contract}}, nil
}

// NewNamespaceCaller creates a new read-only instance of Namespace, bound to a specific deployed contract.
func NewNamespaceCaller(address common.Address, caller bind.ContractCaller) (*NamespaceCaller, error) {
	contract, err := bindNamespace(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &NamespaceCaller{contract: contract}, nil
}

// NewNamespaceTransactor creates a new write-only instance of Namespace, bound to a specific deployed contract.
func NewNamespaceTransactor(address common.Address, transactor bind.ContractTransactor) (*NamespaceTransactor, error) {
	contract, err := bindNamespace(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &NamespaceTransactor{contract: contract}, nil
}

// NewNamespaceFilterer creates a new log filterer instance of Namespace, bound to a specific deployed contract.
func NewNamespaceFilterer(address common.Address, filterer bind.ContractFilterer) (*NamespaceFilterer, error) {
	contract, err := bindNamespace(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &NamespaceFilterer{contract: contract}, nil
}

// bindNamespace binds a generic wrapper to an already deployed contract.
func bindNamespace(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := NamespaceMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Namespace *NamespaceRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Namespace.Contract.NamespaceCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Namespace *NamespaceRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Namespace.Contract.NamespaceTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Namespace *NamespaceRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Namespace.Contract.NamespaceTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Namespace *NamespaceCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Namespace.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Namespace *NamespaceTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Namespace.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Namespace *NamespaceTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Namespace.Contract.contract.Transact(opts, method, params...)
}

// AddressForNamespace is a free data retrieval call binding the contract method 0x0fdf8ad9.
//
// Solidity: function addressForNamespace(string namespace) view returns(string)
func (_Namespace *NamespaceCaller) AddressForNamespace(opts *bind.CallOpts, namespace string) (string, error) {
	var out []interface{}
	err := _Namespace.contract.Call(opts, &out, "addressForNamespace", namespace)

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// AddressForNamespace is a free data retrieval call binding the contract method 0x0fdf8ad9.
//
// Solidity: function addressForNamespace(string namespace) view returns(string)
func (_Namespace *NamespaceSession) AddressForNamespace(namespace string) (string, error) {
	return _Namespace.Contract.AddressForNamespace(&_Namespace.CallOpts, namespace)
}

// AddressForNamespace is a free data retrieval call binding the contract method 0x0fdf8ad9.
//
// Solidity: function addressForNamespace(string namespace) view returns(string)
func (_Namespace *NamespaceCallerSession) AddressForNamespace(namespace string) (string, error) {
	return _Namespace.Contract.AddressForNamespace(&_Namespace.CallOpts, namespace)
}

// Register is a paid mutator transaction binding the contract method 0x3ffbd47f.
//
// Solidity: function register(string namespace, string gRPCAddress) returns(bool)
func (_Namespace *NamespaceTransactor) Register(opts *bind.TransactOpts, namespace string, gRPCAddress string) (*types.Transaction, error) {
	return _Namespace.contract.Transact(opts, "register", namespace, gRPCAddress)
}

// Register is a paid mutator transaction binding the contract method 0x3ffbd47f.
//
// Solidity: function register(string namespace, string gRPCAddress) returns(bool)
func (_Namespace *NamespaceSession) Register(namespace string, gRPCAddress string) (*types.Transaction, error) {
	return _Namespace.Contract.Register(&_Namespace.TransactOpts, namespace, gRPCAddress)
}

// Register is a paid mutator transaction binding the contract method 0x3ffbd47f.
//
// Solidity: function register(string namespace, string gRPCAddress) returns(bool)
func (_Namespace *NamespaceTransactorSession) Register(namespace string, gRPCAddress string) (*types.Transaction, error) {
	return _Namespace.Contract.Register(&_Namespace.TransactOpts, namespace, gRPCAddress)
}
