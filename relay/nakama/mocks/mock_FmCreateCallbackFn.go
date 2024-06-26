// Code generated by mockery v2.23.1. DO NOT EDIT.

package mocks

import (
	runtime "github.com/heroiclabs/nakama-common/runtime"
	mock "github.com/stretchr/testify/mock"
)

// MockFmCreateCallbackFn is an autogenerated mock type for the FmCreateCallbackFn type
type MockFmCreateCallbackFn struct {
	mock.Mock
}

// Execute provides a mock function with given fields: status, instanceInfo, sessionInfo, metadata, err
func (_m *MockFmCreateCallbackFn) Execute(status runtime.FmCreateStatus, instanceInfo *runtime.InstanceInfo, sessionInfo []*runtime.SessionInfo, metadata map[string]interface{}, err error) {
	_m.Called(status, instanceInfo, sessionInfo, metadata, err)
}

type mockConstructorTestingTNewMockFmCreateCallbackFn interface {
	mock.TestingT
	Cleanup(func())
}

// NewMockFmCreateCallbackFn creates a new instance of MockFmCreateCallbackFn. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewMockFmCreateCallbackFn(t mockConstructorTestingTNewMockFmCreateCallbackFn) *MockFmCreateCallbackFn {
	mock := &MockFmCreateCallbackFn{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
