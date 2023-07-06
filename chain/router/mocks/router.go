// Code generated by MockGen. DO NOT EDIT.
// Source: router.go

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	reflect "reflect"

	router "github.com/argus-labs/world-engine/chain/router"
	gomock "github.com/golang/mock/gomock"
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

// Send mocks base method.
func (m *MockRouter) Send(ctx context.Context, namespace, sender, msgID string, msg []byte) (*router.Result, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Send", ctx, namespace, sender, msgID, msg)
	ret0, _ := ret[0].(*router.Result)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Send indicates an expected call of Send.
func (mr *MockRouterMockRecorder) Send(ctx, namespace, sender, msgID, msg interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Send", reflect.TypeOf((*MockRouter)(nil).Send), ctx, namespace, sender, msgID, msg)
}
