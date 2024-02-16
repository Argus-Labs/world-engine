// Code generated by MockGen. DO NOT EDIT.
// Source: context.go

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	zerolog "github.com/rs/zerolog"
	gamestate "pkg.world.dev/world-engine/cardinal/gamestate"
	receipt "pkg.world.dev/world-engine/cardinal/receipt"
	txpool "pkg.world.dev/world-engine/cardinal/txpool"
	types "pkg.world.dev/world-engine/cardinal/types"
	sign "pkg.world.dev/world-engine/sign"
)

// MockContext is a mock of Context interface.
type MockContext struct {
	ctrl     *gomock.Controller
	recorder *MockContextMockRecorder
}

// MockContextMockRecorder is the mock recorder for MockContext.
type MockContextMockRecorder struct {
	mock *MockContext
}

// NewMockContext creates a new mock instance.
func NewMockContext(ctrl *gomock.Controller) *MockContext {
	mock := &MockContext{ctrl: ctrl}
	mock.recorder = &MockContextMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockContext) EXPECT() *MockContextMockRecorder {
	return m.recorder
}

// AddMessageError mocks base method.
func (m *MockContext) AddMessageError(id types.TxHash, err error) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "AddMessageError", id, err)
}

// AddMessageError indicates an expected call of AddMessageError.
func (mr *MockContextMockRecorder) AddMessageError(id, err interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddMessageError", reflect.TypeOf((*MockContext)(nil).AddMessageError), id, err)
}

// AddTransaction mocks base method.
func (m *MockContext) AddTransaction(id types.MessageID, v any, sig *sign.Transaction) (uint64, types.TxHash) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AddTransaction", id, v, sig)
	ret0, _ := ret[0].(uint64)
	ret1, _ := ret[1].(types.TxHash)
	return ret0, ret1
}

// AddTransaction indicates an expected call of AddTransaction.
func (mr *MockContextMockRecorder) AddTransaction(id, v, sig interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddTransaction", reflect.TypeOf((*MockContext)(nil).AddTransaction), id, v, sig)
}

// CurrentTick mocks base method.
func (m *MockContext) CurrentTick() uint64 {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CurrentTick")
	ret0, _ := ret[0].(uint64)
	return ret0
}

// CurrentTick indicates an expected call of CurrentTick.
func (mr *MockContextMockRecorder) CurrentTick() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CurrentTick", reflect.TypeOf((*MockContext)(nil).CurrentTick))
}

// EmitEvent mocks base method.
func (m *MockContext) EmitEvent(arg0 string) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "EmitEvent", arg0)
}

// EmitEvent indicates an expected call of EmitEvent.
func (mr *MockContextMockRecorder) EmitEvent(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "EmitEvent", reflect.TypeOf((*MockContext)(nil).EmitEvent), arg0)
}

// GetComponentByName mocks base method.
func (m *MockContext) GetComponentByName(name string) (types.ComponentMetadata, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetComponentByName", name)
	ret0, _ := ret[0].(types.ComponentMetadata)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetComponentByName indicates an expected call of GetComponentByName.
func (mr *MockContextMockRecorder) GetComponentByName(name interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetComponentByName", reflect.TypeOf((*MockContext)(nil).GetComponentByName), name)
}

// GetSignerForPersonaTag mocks base method.
func (m *MockContext) GetSignerForPersonaTag(personaTag string, tick uint64) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetSignerForPersonaTag", personaTag, tick)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetSignerForPersonaTag indicates an expected call of GetSignerForPersonaTag.
func (mr *MockContextMockRecorder) GetSignerForPersonaTag(personaTag, tick interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetSignerForPersonaTag", reflect.TypeOf((*MockContext)(nil).GetSignerForPersonaTag), personaTag, tick)
}

// GetTransactionReceipt mocks base method.
func (m *MockContext) GetTransactionReceipt(id types.TxHash) (any, []error, bool) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetTransactionReceipt", id)
	ret0, _ := ret[0].(any)
	ret1, _ := ret[1].([]error)
	ret2, _ := ret[2].(bool)
	return ret0, ret1, ret2
}

// GetTransactionReceipt indicates an expected call of GetTransactionReceipt.
func (mr *MockContextMockRecorder) GetTransactionReceipt(id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetTransactionReceipt", reflect.TypeOf((*MockContext)(nil).GetTransactionReceipt), id)
}

// GetTransactionReceiptsForTick mocks base method.
func (m *MockContext) GetTransactionReceiptsForTick(tick uint64) ([]receipt.Receipt, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetTransactionReceiptsForTick", tick)
	ret0, _ := ret[0].([]receipt.Receipt)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetTransactionReceiptsForTick indicates an expected call of GetTransactionReceiptsForTick.
func (mr *MockContextMockRecorder) GetTransactionReceiptsForTick(tick interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetTransactionReceiptsForTick", reflect.TypeOf((*MockContext)(nil).GetTransactionReceiptsForTick), tick)
}

// GetTxQueue mocks base method.
func (m *MockContext) GetTxQueue() *txpool.TxQueue {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetTxQueue")
	ret0, _ := ret[0].(*txpool.TxQueue)
	return ret0
}

// GetTxQueue indicates an expected call of GetTxQueue.
func (mr *MockContextMockRecorder) GetTxQueue() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetTxQueue", reflect.TypeOf((*MockContext)(nil).GetTxQueue))
}

// IsReadOnly mocks base method.
func (m *MockContext) IsReadOnly() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsReadOnly")
	ret0, _ := ret[0].(bool)
	return ret0
}

// IsReadOnly indicates an expected call of IsReadOnly.
func (mr *MockContextMockRecorder) IsReadOnly() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsReadOnly", reflect.TypeOf((*MockContext)(nil).IsReadOnly))
}

// IsWorldReady mocks base method.
func (m *MockContext) IsWorldReady() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsWorldReady")
	ret0, _ := ret[0].(bool)
	return ret0
}

// IsWorldReady indicates an expected call of IsWorldReady.
func (mr *MockContextMockRecorder) IsWorldReady() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsWorldReady", reflect.TypeOf((*MockContext)(nil).IsWorldReady))
}

// Logger mocks base method.
func (m *MockContext) Logger() *zerolog.Logger {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Logger")
	ret0, _ := ret[0].(*zerolog.Logger)
	return ret0
}

// Logger indicates an expected call of Logger.
func (mr *MockContextMockRecorder) Logger() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Logger", reflect.TypeOf((*MockContext)(nil).Logger))
}

// Namespace mocks base method.
func (m *MockContext) Namespace() string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Namespace")
	ret0, _ := ret[0].(string)
	return ret0
}

// Namespace indicates an expected call of Namespace.
func (mr *MockContextMockRecorder) Namespace() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Namespace", reflect.TypeOf((*MockContext)(nil).Namespace))
}

// ReceiptHistorySize mocks base method.
func (m *MockContext) ReceiptHistorySize() uint64 {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReceiptHistorySize")
	ret0, _ := ret[0].(uint64)
	return ret0
}

// ReceiptHistorySize indicates an expected call of ReceiptHistorySize.
func (mr *MockContextMockRecorder) ReceiptHistorySize() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReceiptHistorySize", reflect.TypeOf((*MockContext)(nil).ReceiptHistorySize))
}

// SetLogger mocks base method.
func (m *MockContext) SetLogger(logger zerolog.Logger) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "SetLogger", logger)
}

// SetLogger indicates an expected call of SetLogger.
func (mr *MockContextMockRecorder) SetLogger(logger interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetLogger", reflect.TypeOf((*MockContext)(nil).SetLogger), logger)
}

// SetMessageResult mocks base method.
func (m *MockContext) SetMessageResult(id types.TxHash, a any) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "SetMessageResult", id, a)
}

// SetMessageResult indicates an expected call of SetMessageResult.
func (mr *MockContextMockRecorder) SetMessageResult(id, a interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetMessageResult", reflect.TypeOf((*MockContext)(nil).SetMessageResult), id, a)
}

// StoreManager mocks base method.
func (m *MockContext) StoreManager() gamestate.Manager {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "StoreManager")
	ret0, _ := ret[0].(gamestate.Manager)
	return ret0
}

// StoreManager indicates an expected call of StoreManager.
func (mr *MockContextMockRecorder) StoreManager() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "StoreManager", reflect.TypeOf((*MockContext)(nil).StoreManager))
}

// StoreReader mocks base method.
func (m *MockContext) StoreReader() gamestate.Reader {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "StoreReader")
	ret0, _ := ret[0].(gamestate.Reader)
	return ret0
}

// StoreReader indicates an expected call of StoreReader.
func (mr *MockContextMockRecorder) StoreReader() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "StoreReader", reflect.TypeOf((*MockContext)(nil).StoreReader))
}

// Timestamp mocks base method.
func (m *MockContext) Timestamp() uint64 {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Timestamp")
	ret0, _ := ret[0].(uint64)
	return ret0
}

// Timestamp indicates an expected call of Timestamp.
func (mr *MockContextMockRecorder) Timestamp() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Timestamp", reflect.TypeOf((*MockContext)(nil).Timestamp))
}

// UseNonce mocks base method.
func (m *MockContext) UseNonce(signerAddress string, nonce uint64) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UseNonce", signerAddress, nonce)
	ret0, _ := ret[0].(error)
	return ret0
}

// UseNonce indicates an expected call of UseNonce.
func (mr *MockContextMockRecorder) UseNonce(signerAddress, nonce interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UseNonce", reflect.TypeOf((*MockContext)(nil).UseNonce), signerAddress, nonce)
}
