// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package argus

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
)

// QuestMetaData contains all meta data concerning the Quest contract.
var QuestMetaData = &bind.MetaData{
	ABI: "[{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"QuestComplete\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"a\",\"type\":\"address\"}],\"name\":\"completeQuest\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
}

// QuestABI is the input ABI used to generate the binding from.
// Deprecated: Use QuestMetaData.ABI instead.
var QuestABI = QuestMetaData.ABI

// Quest is an auto generated Go binding around an Ethereum contract.
type Quest struct {
	QuestCaller     // Read-only binding to the contract
	QuestTransactor // Write-only binding to the contract
	QuestFilterer   // Log filterer for contract events
}

// QuestCaller is an auto generated read-only Go binding around an Ethereum contract.
type QuestCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// QuestTransactor is an auto generated write-only Go binding around an Ethereum contract.
type QuestTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// QuestFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type QuestFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// QuestSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type QuestSession struct {
	Contract     *Quest            // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// QuestCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type QuestCallerSession struct {
	Contract *QuestCaller  // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts // Call options to use throughout this session
}

// QuestTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type QuestTransactorSession struct {
	Contract     *QuestTransactor  // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// QuestRaw is an auto generated low-level Go binding around an Ethereum contract.
type QuestRaw struct {
	Contract *Quest // Generic contract binding to access the raw methods on
}

// QuestCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type QuestCallerRaw struct {
	Contract *QuestCaller // Generic read-only contract binding to access the raw methods on
}

// QuestTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type QuestTransactorRaw struct {
	Contract *QuestTransactor // Generic write-only contract binding to access the raw methods on
}

// NewQuest creates a new instance of Quest, bound to a specific deployed contract.
func NewQuest(address common.Address, backend bind.ContractBackend) (*Quest, error) {
	contract, err := bindQuest(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Quest{QuestCaller: QuestCaller{contract: contract}, QuestTransactor: QuestTransactor{contract: contract}, QuestFilterer: QuestFilterer{contract: contract}}, nil
}

// NewQuestCaller creates a new read-only instance of Quest, bound to a specific deployed contract.
func NewQuestCaller(address common.Address, caller bind.ContractCaller) (*QuestCaller, error) {
	contract, err := bindQuest(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &QuestCaller{contract: contract}, nil
}

// NewQuestTransactor creates a new write-only instance of Quest, bound to a specific deployed contract.
func NewQuestTransactor(address common.Address, transactor bind.ContractTransactor) (*QuestTransactor, error) {
	contract, err := bindQuest(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &QuestTransactor{contract: contract}, nil
}

// NewQuestFilterer creates a new log filterer instance of Quest, bound to a specific deployed contract.
func NewQuestFilterer(address common.Address, filterer bind.ContractFilterer) (*QuestFilterer, error) {
	contract, err := bindQuest(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &QuestFilterer{contract: contract}, nil
}

// bindQuest binds a generic wrapper to an already deployed contract.
func bindQuest(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(QuestABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Quest *QuestRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Quest.Contract.QuestCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Quest *QuestRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Quest.Contract.QuestTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Quest *QuestRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Quest.Contract.QuestTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Quest *QuestCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Quest.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Quest *QuestTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Quest.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Quest *QuestTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Quest.Contract.contract.Transact(opts, method, params...)
}

// CompleteQuest is a paid mutator transaction binding the contract method 0x77b37fe7.
//
// Solidity: function completeQuest(address a) returns()
func (_Quest *QuestTransactor) CompleteQuest(opts *bind.TransactOpts, a common.Address) (*types.Transaction, error) {
	return _Quest.contract.Transact(opts, "completeQuest", a)
}

// CompleteQuest is a paid mutator transaction binding the contract method 0x77b37fe7.
//
// Solidity: function completeQuest(address a) returns()
func (_Quest *QuestSession) CompleteQuest(a common.Address) (*types.Transaction, error) {
	return _Quest.Contract.CompleteQuest(&_Quest.TransactOpts, a)
}

// CompleteQuest is a paid mutator transaction binding the contract method 0x77b37fe7.
//
// Solidity: function completeQuest(address a) returns()
func (_Quest *QuestTransactorSession) CompleteQuest(a common.Address) (*types.Transaction, error) {
	return _Quest.Contract.CompleteQuest(&_Quest.TransactOpts, a)
}

// QuestQuestCompleteIterator is returned from FilterQuestComplete and is used to iterate over the raw logs and unpacked data for QuestComplete events raised by the Quest contract.
type QuestQuestCompleteIterator struct {
	Event *QuestQuestComplete // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *QuestQuestCompleteIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(QuestQuestComplete)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(QuestQuestComplete)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *QuestQuestCompleteIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *QuestQuestCompleteIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// QuestQuestComplete represents a QuestComplete event raised by the Quest contract.
type QuestQuestComplete struct {
	Arg0 common.Address
	Raw  types.Log // Blockchain specific contextual infos
}

// FilterQuestComplete is a free log retrieval operation binding the contract event 0xadf42909b380f9140633e3b84d758a4ffd81c45e18e5647f7636a8674012e9ed.
//
// Solidity: event QuestComplete(address arg0)
func (_Quest *QuestFilterer) FilterQuestComplete(opts *bind.FilterOpts) (*QuestQuestCompleteIterator, error) {

	logs, sub, err := _Quest.contract.FilterLogs(opts, "QuestComplete")
	if err != nil {
		return nil, err
	}
	return &QuestQuestCompleteIterator{contract: _Quest.contract, event: "QuestComplete", logs: logs, sub: sub}, nil
}

// WatchQuestComplete is a free log subscription operation binding the contract event 0xadf42909b380f9140633e3b84d758a4ffd81c45e18e5647f7636a8674012e9ed.
//
// Solidity: event QuestComplete(address arg0)
func (_Quest *QuestFilterer) WatchQuestComplete(opts *bind.WatchOpts, sink chan<- *QuestQuestComplete) (event.Subscription, error) {

	logs, sub, err := _Quest.contract.WatchLogs(opts, "QuestComplete")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(QuestQuestComplete)
				if err := _Quest.contract.UnpackLog(event, "QuestComplete", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseQuestComplete is a log parse operation binding the contract event 0xadf42909b380f9140633e3b84d758a4ffd81c45e18e5647f7636a8674012e9ed.
//
// Solidity: event QuestComplete(address arg0)
func (_Quest *QuestFilterer) ParseQuestComplete(log types.Log) (*QuestQuestComplete, error) {
	event := new(QuestQuestComplete)
	if err := _Quest.contract.UnpackLog(event, "QuestComplete", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
