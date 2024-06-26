// Code generated by mockery v2.23.1. DO NOT EDIT.

package mocks

import (
	runtime "github.com/heroiclabs/nakama-common/runtime"
	mock "github.com/stretchr/testify/mock"
)

// MockPresenceMeta is an autogenerated mock type for the PresenceMeta type
type MockPresenceMeta struct {
	mock.Mock
}

// GetHidden provides a mock function with given fields:
func (_m *MockPresenceMeta) GetHidden() bool {
	ret := _m.Called()

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// GetPersistence provides a mock function with given fields:
func (_m *MockPresenceMeta) GetPersistence() bool {
	ret := _m.Called()

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// GetReason provides a mock function with given fields:
func (_m *MockPresenceMeta) GetReason() runtime.PresenceReason {
	ret := _m.Called()

	var r0 runtime.PresenceReason
	if rf, ok := ret.Get(0).(func() runtime.PresenceReason); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(runtime.PresenceReason)
	}

	return r0
}

// GetStatus provides a mock function with given fields:
func (_m *MockPresenceMeta) GetStatus() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// GetUsername provides a mock function with given fields:
func (_m *MockPresenceMeta) GetUsername() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

type mockConstructorTestingTNewMockPresenceMeta interface {
	mock.TestingT
	Cleanup(func())
}

// NewMockPresenceMeta creates a new instance of MockPresenceMeta. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewMockPresenceMeta(t mockConstructorTestingTNewMockPresenceMeta) *MockPresenceMeta {
	mock := &MockPresenceMeta{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
