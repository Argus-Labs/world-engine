// Code generated by mockery v2.23.1. DO NOT EDIT.

package mocks

import (
	runtime "github.com/heroiclabs/nakama-common/runtime"
	mock "github.com/stretchr/testify/mock"
)

// MockMatchmakerEntry is an autogenerated mock type for the MatchmakerEntry type
type MockMatchmakerEntry struct {
	mock.Mock
}

// GetPartyId provides a mock function with given fields:
func (_m *MockMatchmakerEntry) GetPartyId() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// GetPresence provides a mock function with given fields:
func (_m *MockMatchmakerEntry) GetPresence() runtime.Presence {
	ret := _m.Called()

	var r0 runtime.Presence
	if rf, ok := ret.Get(0).(func() runtime.Presence); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(runtime.Presence)
		}
	}

	return r0
}

// GetProperties provides a mock function with given fields:
func (_m *MockMatchmakerEntry) GetProperties() map[string]interface{} {
	ret := _m.Called()

	var r0 map[string]interface{}
	if rf, ok := ret.Get(0).(func() map[string]interface{}); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(map[string]interface{})
		}
	}

	return r0
}

// GetTicket provides a mock function with given fields:
func (_m *MockMatchmakerEntry) GetTicket() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

type mockConstructorTestingTNewMockMatchmakerEntry interface {
	mock.TestingT
	Cleanup(func())
}

// NewMockMatchmakerEntry creates a new instance of MockMatchmakerEntry. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewMockMatchmakerEntry(t mockConstructorTestingTNewMockMatchmakerEntry) *MockMatchmakerEntry {
	mock := &MockMatchmakerEntry{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
