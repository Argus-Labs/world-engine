// Code generated by MockGen. DO NOT EDIT.
// Source: provider.go

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	component "pkg.world.dev/world-engine/cardinal/persona/component"
	types "pkg.world.dev/world-engine/cardinal/types"
	sign "pkg.world.dev/world-engine/sign"
)

// MockProvider is a mock of Provider interface.
type MockProvider struct {
	ctrl     *gomock.Controller
	recorder *MockProviderMockRecorder
}

// MockProviderMockRecorder is the mock recorder for MockProvider.
type MockProviderMockRecorder struct {
	mock *MockProvider
}

// NewMockProvider creates a new mock instance.
func NewMockProvider(ctrl *gomock.Controller) *MockProvider {
	mock := &MockProvider{ctrl: ctrl}
	mock.recorder = &MockProviderMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockProvider) EXPECT() *MockProviderMockRecorder {
	return m.recorder
}

// AddEVMTransaction mocks base method.
func (m *MockProvider) AddEVMTransaction(id types.MessageID, msgValue any, tx *sign.Transaction, evmTxHash string) (uint64, types.TxHash) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AddEVMTransaction", id, msgValue, tx, evmTxHash)
	ret0, _ := ret[0].(uint64)
	ret1, _ := ret[1].(types.TxHash)
	return ret0, ret1
}

// AddEVMTransaction indicates an expected call of AddEVMTransaction.
func (mr *MockProviderMockRecorder) AddEVMTransaction(id, msgValue, tx, evmTxHash interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddEVMTransaction", reflect.TypeOf((*MockProvider)(nil).AddEVMTransaction), id, msgValue, tx, evmTxHash)
}

// ConsumeEVMMsgResult mocks base method.
func (m *MockProvider) ConsumeEVMMsgResult(evmTxHash string) ([]byte, []error, string, bool) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ConsumeEVMMsgResult", evmTxHash)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].([]error)
	ret2, _ := ret[2].(string)
	ret3, _ := ret[3].(bool)
	return ret0, ret1, ret2, ret3
}

// ConsumeEVMMsgResult indicates an expected call of ConsumeEVMMsgResult.
func (mr *MockProviderMockRecorder) ConsumeEVMMsgResult(evmTxHash interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ConsumeEVMMsgResult", reflect.TypeOf((*MockProvider)(nil).ConsumeEVMMsgResult), evmTxHash)
}

// GetMessageByID mocks base method.
func (m *MockProvider) GetMessageByID(id types.MessageID) (types.Message, bool) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetMessageByID", id)
	ret0, _ := ret[0].(types.Message)
	ret1, _ := ret[1].(bool)
	return ret0, ret1
}

// GetMessageByID indicates an expected call of GetMessageByID.
func (mr *MockProviderMockRecorder) GetMessageByID(id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetMessageByID", reflect.TypeOf((*MockProvider)(nil).GetMessageByID), id)
}

// GetMessageByName mocks base method.
func (m *MockProvider) GetMessageByName(arg0 string) (types.Message, bool) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetMessageByName", arg0)
	ret0, _ := ret[0].(types.Message)
	ret1, _ := ret[1].(bool)
	return ret0, ret1
}

// GetMessageByName indicates an expected call of GetMessageByName.
func (mr *MockProviderMockRecorder) GetMessageByName(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetMessageByName", reflect.TypeOf((*MockProvider)(nil).GetMessageByName), arg0)
}

// GetSignerComponentForPersona mocks base method.
func (m *MockProvider) GetSignerComponentForPersona(arg0 string) (*component.SignerComponent, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetSignerComponentForPersona", arg0)
	ret0, _ := ret[0].(*component.SignerComponent)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetSignerComponentForPersona indicates an expected call of GetSignerComponentForPersona.
func (mr *MockProviderMockRecorder) GetSignerComponentForPersona(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetSignerComponentForPersona", reflect.TypeOf((*MockProvider)(nil).GetSignerComponentForPersona), arg0)
}

// HandleEVMQuery mocks base method.
func (m *MockProvider) HandleEVMQuery(name string, abiRequest []byte) ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HandleEVMQuery", name, abiRequest)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// HandleEVMQuery indicates an expected call of HandleEVMQuery.
func (mr *MockProviderMockRecorder) HandleEVMQuery(name, abiRequest interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "HandleEVMQuery", reflect.TypeOf((*MockProvider)(nil).HandleEVMQuery), name, abiRequest)
}

// WaitForNextTick mocks base method.
func (m *MockProvider) WaitForNextTick() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "WaitForNextTick")
	ret0, _ := ret[0].(bool)
	return ret0
}

// WaitForNextTick indicates an expected call of WaitForNextTick.
func (mr *MockProviderMockRecorder) WaitForNextTick() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "WaitForNextTick", reflect.TypeOf((*MockProvider)(nil).WaitForNextTick))
}
