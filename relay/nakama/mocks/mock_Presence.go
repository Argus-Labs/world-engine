// Code generated by mockery v2.38.0. DO NOT EDIT.

package mocks

import (
	runtime "github.com/heroiclabs/nakama-common/runtime"
	mock "github.com/stretchr/testify/mock"
)

// Presence is an autogenerated mock type for the Presence type
type Presence struct {
	mock.Mock
}

type Presence_Expecter struct {
	mock *mock.Mock
}

func (_m *Presence) EXPECT() *Presence_Expecter {
	return &Presence_Expecter{mock: &_m.Mock}
}

// GetHidden provides a mock function with given fields:
func (_m *Presence) GetHidden() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetHidden")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// Presence_GetHidden_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetHidden'
type Presence_GetHidden_Call struct {
	*mock.Call
}

// GetHidden is a helper method to define mock.On call
func (_e *Presence_Expecter) GetHidden() *Presence_GetHidden_Call {
	return &Presence_GetHidden_Call{Call: _e.mock.On("GetHidden")}
}

func (_c *Presence_GetHidden_Call) Run(run func()) *Presence_GetHidden_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *Presence_GetHidden_Call) Return(_a0 bool) *Presence_GetHidden_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *Presence_GetHidden_Call) RunAndReturn(run func() bool) *Presence_GetHidden_Call {
	_c.Call.Return(run)
	return _c
}

// GetNodeId provides a mock function with given fields:
func (_m *Presence) GetNodeId() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetNodeId")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// Presence_GetNodeId_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetNodeId'
type Presence_GetNodeId_Call struct {
	*mock.Call
}

// GetNodeId is a helper method to define mock.On call
func (_e *Presence_Expecter) GetNodeId() *Presence_GetNodeId_Call {
	return &Presence_GetNodeId_Call{Call: _e.mock.On("GetNodeId")}
}

func (_c *Presence_GetNodeId_Call) Run(run func()) *Presence_GetNodeId_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *Presence_GetNodeId_Call) Return(_a0 string) *Presence_GetNodeId_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *Presence_GetNodeId_Call) RunAndReturn(run func() string) *Presence_GetNodeId_Call {
	_c.Call.Return(run)
	return _c
}

// GetPersistence provides a mock function with given fields:
func (_m *Presence) GetPersistence() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetPersistence")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// Presence_GetPersistence_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetPersistence'
type Presence_GetPersistence_Call struct {
	*mock.Call
}

// GetPersistence is a helper method to define mock.On call
func (_e *Presence_Expecter) GetPersistence() *Presence_GetPersistence_Call {
	return &Presence_GetPersistence_Call{Call: _e.mock.On("GetPersistence")}
}

func (_c *Presence_GetPersistence_Call) Run(run func()) *Presence_GetPersistence_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *Presence_GetPersistence_Call) Return(_a0 bool) *Presence_GetPersistence_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *Presence_GetPersistence_Call) RunAndReturn(run func() bool) *Presence_GetPersistence_Call {
	_c.Call.Return(run)
	return _c
}

// GetReason provides a mock function with given fields:
func (_m *Presence) GetReason() runtime.PresenceReason {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetReason")
	}

	var r0 runtime.PresenceReason
	if rf, ok := ret.Get(0).(func() runtime.PresenceReason); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(runtime.PresenceReason)
	}

	return r0
}

// Presence_GetReason_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetReason'
type Presence_GetReason_Call struct {
	*mock.Call
}

// GetReason is a helper method to define mock.On call
func (_e *Presence_Expecter) GetReason() *Presence_GetReason_Call {
	return &Presence_GetReason_Call{Call: _e.mock.On("GetReason")}
}

func (_c *Presence_GetReason_Call) Run(run func()) *Presence_GetReason_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *Presence_GetReason_Call) Return(_a0 runtime.PresenceReason) *Presence_GetReason_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *Presence_GetReason_Call) RunAndReturn(run func() runtime.PresenceReason) *Presence_GetReason_Call {
	_c.Call.Return(run)
	return _c
}

// GetSessionId provides a mock function with given fields:
func (_m *Presence) GetSessionId() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetSessionId")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// Presence_GetSessionId_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetSessionId'
type Presence_GetSessionId_Call struct {
	*mock.Call
}

// GetSessionId is a helper method to define mock.On call
func (_e *Presence_Expecter) GetSessionId() *Presence_GetSessionId_Call {
	return &Presence_GetSessionId_Call{Call: _e.mock.On("GetSessionId")}
}

func (_c *Presence_GetSessionId_Call) Run(run func()) *Presence_GetSessionId_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *Presence_GetSessionId_Call) Return(_a0 string) *Presence_GetSessionId_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *Presence_GetSessionId_Call) RunAndReturn(run func() string) *Presence_GetSessionId_Call {
	_c.Call.Return(run)
	return _c
}

// GetStatus provides a mock function with given fields:
func (_m *Presence) GetStatus() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetStatus")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// Presence_GetStatus_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetStatus'
type Presence_GetStatus_Call struct {
	*mock.Call
}

// GetStatus is a helper method to define mock.On call
func (_e *Presence_Expecter) GetStatus() *Presence_GetStatus_Call {
	return &Presence_GetStatus_Call{Call: _e.mock.On("GetStatus")}
}

func (_c *Presence_GetStatus_Call) Run(run func()) *Presence_GetStatus_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *Presence_GetStatus_Call) Return(_a0 string) *Presence_GetStatus_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *Presence_GetStatus_Call) RunAndReturn(run func() string) *Presence_GetStatus_Call {
	_c.Call.Return(run)
	return _c
}

// GetUserId provides a mock function with given fields:
func (_m *Presence) GetUserId() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetUserId")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// Presence_GetUserId_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetUserId'
type Presence_GetUserId_Call struct {
	*mock.Call
}

// GetUserId is a helper method to define mock.On call
func (_e *Presence_Expecter) GetUserId() *Presence_GetUserId_Call {
	return &Presence_GetUserId_Call{Call: _e.mock.On("GetUserId")}
}

func (_c *Presence_GetUserId_Call) Run(run func()) *Presence_GetUserId_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *Presence_GetUserId_Call) Return(_a0 string) *Presence_GetUserId_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *Presence_GetUserId_Call) RunAndReturn(run func() string) *Presence_GetUserId_Call {
	_c.Call.Return(run)
	return _c
}

// GetUsername provides a mock function with given fields:
func (_m *Presence) GetUsername() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetUsername")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// Presence_GetUsername_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetUsername'
type Presence_GetUsername_Call struct {
	*mock.Call
}

// GetUsername is a helper method to define mock.On call
func (_e *Presence_Expecter) GetUsername() *Presence_GetUsername_Call {
	return &Presence_GetUsername_Call{Call: _e.mock.On("GetUsername")}
}

func (_c *Presence_GetUsername_Call) Run(run func()) *Presence_GetUsername_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *Presence_GetUsername_Call) Return(_a0 string) *Presence_GetUsername_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *Presence_GetUsername_Call) RunAndReturn(run func() string) *Presence_GetUsername_Call {
	_c.Call.Return(run)
	return _c
}

// NewPresence creates a new instance of Presence. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewPresence(t interface {
	mock.TestingT
	Cleanup(func())
}) *Presence {
	mock := &Presence{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
