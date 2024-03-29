// Code generated by mockery v2.38.0. DO NOT EDIT.

package mocks

import (
	runtime "github.com/heroiclabs/nakama-common/runtime"
	mock "github.com/stretchr/testify/mock"
)

// MatchmakerEntry is an autogenerated mock type for the MatchmakerEntry type
type MatchmakerEntry struct {
	mock.Mock
}

type MatchmakerEntry_Expecter struct {
	mock *mock.Mock
}

func (_m *MatchmakerEntry) EXPECT() *MatchmakerEntry_Expecter {
	return &MatchmakerEntry_Expecter{mock: &_m.Mock}
}

// GetPartyId provides a mock function with given fields:
func (_m *MatchmakerEntry) GetPartyId() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetPartyId")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// MatchmakerEntry_GetPartyId_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetPartyId'
type MatchmakerEntry_GetPartyId_Call struct {
	*mock.Call
}

// GetPartyId is a helper method to define mock.On call
func (_e *MatchmakerEntry_Expecter) GetPartyId() *MatchmakerEntry_GetPartyId_Call {
	return &MatchmakerEntry_GetPartyId_Call{Call: _e.mock.On("GetPartyId")}
}

func (_c *MatchmakerEntry_GetPartyId_Call) Run(run func()) *MatchmakerEntry_GetPartyId_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MatchmakerEntry_GetPartyId_Call) Return(_a0 string) *MatchmakerEntry_GetPartyId_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MatchmakerEntry_GetPartyId_Call) RunAndReturn(run func() string) *MatchmakerEntry_GetPartyId_Call {
	_c.Call.Return(run)
	return _c
}

// GetPresence provides a mock function with given fields:
func (_m *MatchmakerEntry) GetPresence() runtime.Presence {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetPresence")
	}

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

// MatchmakerEntry_GetPresence_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetPresence'
type MatchmakerEntry_GetPresence_Call struct {
	*mock.Call
}

// GetPresence is a helper method to define mock.On call
func (_e *MatchmakerEntry_Expecter) GetPresence() *MatchmakerEntry_GetPresence_Call {
	return &MatchmakerEntry_GetPresence_Call{Call: _e.mock.On("GetPresence")}
}

func (_c *MatchmakerEntry_GetPresence_Call) Run(run func()) *MatchmakerEntry_GetPresence_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MatchmakerEntry_GetPresence_Call) Return(_a0 runtime.Presence) *MatchmakerEntry_GetPresence_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MatchmakerEntry_GetPresence_Call) RunAndReturn(run func() runtime.Presence) *MatchmakerEntry_GetPresence_Call {
	_c.Call.Return(run)
	return _c
}

// GetProperties provides a mock function with given fields:
func (_m *MatchmakerEntry) GetProperties() map[string]interface{} {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetProperties")
	}

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

// MatchmakerEntry_GetProperties_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetProperties'
type MatchmakerEntry_GetProperties_Call struct {
	*mock.Call
}

// GetProperties is a helper method to define mock.On call
func (_e *MatchmakerEntry_Expecter) GetProperties() *MatchmakerEntry_GetProperties_Call {
	return &MatchmakerEntry_GetProperties_Call{Call: _e.mock.On("GetProperties")}
}

func (_c *MatchmakerEntry_GetProperties_Call) Run(run func()) *MatchmakerEntry_GetProperties_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MatchmakerEntry_GetProperties_Call) Return(_a0 map[string]interface{}) *MatchmakerEntry_GetProperties_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MatchmakerEntry_GetProperties_Call) RunAndReturn(run func() map[string]interface{}) *MatchmakerEntry_GetProperties_Call {
	_c.Call.Return(run)
	return _c
}

// GetTicket provides a mock function with given fields:
func (_m *MatchmakerEntry) GetTicket() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetTicket")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// MatchmakerEntry_GetTicket_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetTicket'
type MatchmakerEntry_GetTicket_Call struct {
	*mock.Call
}

// GetTicket is a helper method to define mock.On call
func (_e *MatchmakerEntry_Expecter) GetTicket() *MatchmakerEntry_GetTicket_Call {
	return &MatchmakerEntry_GetTicket_Call{Call: _e.mock.On("GetTicket")}
}

func (_c *MatchmakerEntry_GetTicket_Call) Run(run func()) *MatchmakerEntry_GetTicket_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MatchmakerEntry_GetTicket_Call) Return(_a0 string) *MatchmakerEntry_GetTicket_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MatchmakerEntry_GetTicket_Call) RunAndReturn(run func() string) *MatchmakerEntry_GetTicket_Call {
	_c.Call.Return(run)
	return _c
}

// NewMatchmakerEntry creates a new instance of MatchmakerEntry. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMatchmakerEntry(t interface {
	mock.TestingT
	Cleanup(func())
}) *MatchmakerEntry {
	mock := &MatchmakerEntry{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
