// Code generated by MockGen. DO NOT EDIT.
// Source: router.go

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	txpool "pkg.world.dev/world-engine/cardinal/txpool"
	types "pkg.world.dev/world-engine/evm/x/shard/types"
)

// MockRouter is a mock of Router interface.
type MockRouter struct {
	ctrl     *gomock.Controller
	recorder *MockRouterMockRecorder
}

// MockRouterMockRecorder is the mock recorder for MockRouter.
type MockRouterMockRecorder struct {
	mock *MockRouter
}

// NewMockRouter creates a new mock instance.
func NewMockRouter(ctrl *gomock.Controller) *MockRouter {
	mock := &MockRouter{ctrl: ctrl}
	mock.recorder = &MockRouterMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockRouter) EXPECT() *MockRouterMockRecorder {
	return m.recorder
}

// QueryTransactions mocks base method.
func (m *MockRouter) QueryTransactions(ctx context.Context, req *types.QueryTransactionsRequest) (*types.QueryTransactionsResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "QueryTransactions", ctx, req)
	ret0, _ := ret[0].(*types.QueryTransactionsResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// QueryTransactions indicates an expected call of QueryTransactions.
func (mr *MockRouterMockRecorder) QueryTransactions(ctx, req interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "QueryTransactions", reflect.TypeOf((*MockRouter)(nil).QueryTransactions), ctx, req)
}

// Shutdown mocks base method.
func (m *MockRouter) Shutdown() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Shutdown")
}

// Shutdown indicates an expected call of Shutdown.
func (mr *MockRouterMockRecorder) Shutdown() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Shutdown", reflect.TypeOf((*MockRouter)(nil).Shutdown))
}

// Start mocks base method.
func (m *MockRouter) Start() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Start")
	ret0, _ := ret[0].(error)
	return ret0
}

// Start indicates an expected call of Start.
func (mr *MockRouterMockRecorder) Start() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Start", reflect.TypeOf((*MockRouter)(nil).Start))
}

// SubmitTxBlob mocks base method.
func (m *MockRouter) SubmitTxBlob(ctx context.Context, processedTxs txpool.TxMap, epoch, unixTimestamp uint64) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SubmitTxBlob", ctx, processedTxs, epoch, unixTimestamp)
	ret0, _ := ret[0].(error)
	return ret0
}

// SubmitTxBlob indicates an expected call of SubmitTxBlob.
func (mr *MockRouterMockRecorder) SubmitTxBlob(ctx, processedTxs, epoch, unixTimestamp interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SubmitTxBlob", reflect.TypeOf((*MockRouter)(nil).SubmitTxBlob), ctx, processedTxs, epoch, unixTimestamp)
}
