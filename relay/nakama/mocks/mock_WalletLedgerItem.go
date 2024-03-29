// Code generated by mockery v2.38.0. DO NOT EDIT.

package mocks

import mock "github.com/stretchr/testify/mock"

// WalletLedgerItem is an autogenerated mock type for the WalletLedgerItem type
type WalletLedgerItem struct {
	mock.Mock
}

type WalletLedgerItem_Expecter struct {
	mock *mock.Mock
}

func (_m *WalletLedgerItem) EXPECT() *WalletLedgerItem_Expecter {
	return &WalletLedgerItem_Expecter{mock: &_m.Mock}
}

// GetChangeset provides a mock function with given fields:
func (_m *WalletLedgerItem) GetChangeset() map[string]int64 {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetChangeset")
	}

	var r0 map[string]int64
	if rf, ok := ret.Get(0).(func() map[string]int64); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(map[string]int64)
		}
	}

	return r0
}

// WalletLedgerItem_GetChangeset_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetChangeset'
type WalletLedgerItem_GetChangeset_Call struct {
	*mock.Call
}

// GetChangeset is a helper method to define mock.On call
func (_e *WalletLedgerItem_Expecter) GetChangeset() *WalletLedgerItem_GetChangeset_Call {
	return &WalletLedgerItem_GetChangeset_Call{Call: _e.mock.On("GetChangeset")}
}

func (_c *WalletLedgerItem_GetChangeset_Call) Run(run func()) *WalletLedgerItem_GetChangeset_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *WalletLedgerItem_GetChangeset_Call) Return(_a0 map[string]int64) *WalletLedgerItem_GetChangeset_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *WalletLedgerItem_GetChangeset_Call) RunAndReturn(run func() map[string]int64) *WalletLedgerItem_GetChangeset_Call {
	_c.Call.Return(run)
	return _c
}

// GetCreateTime provides a mock function with given fields:
func (_m *WalletLedgerItem) GetCreateTime() int64 {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetCreateTime")
	}

	var r0 int64
	if rf, ok := ret.Get(0).(func() int64); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int64)
	}

	return r0
}

// WalletLedgerItem_GetCreateTime_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetCreateTime'
type WalletLedgerItem_GetCreateTime_Call struct {
	*mock.Call
}

// GetCreateTime is a helper method to define mock.On call
func (_e *WalletLedgerItem_Expecter) GetCreateTime() *WalletLedgerItem_GetCreateTime_Call {
	return &WalletLedgerItem_GetCreateTime_Call{Call: _e.mock.On("GetCreateTime")}
}

func (_c *WalletLedgerItem_GetCreateTime_Call) Run(run func()) *WalletLedgerItem_GetCreateTime_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *WalletLedgerItem_GetCreateTime_Call) Return(_a0 int64) *WalletLedgerItem_GetCreateTime_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *WalletLedgerItem_GetCreateTime_Call) RunAndReturn(run func() int64) *WalletLedgerItem_GetCreateTime_Call {
	_c.Call.Return(run)
	return _c
}

// GetID provides a mock function with given fields:
func (_m *WalletLedgerItem) GetID() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetID")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// WalletLedgerItem_GetID_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetID'
type WalletLedgerItem_GetID_Call struct {
	*mock.Call
}

// GetID is a helper method to define mock.On call
func (_e *WalletLedgerItem_Expecter) GetID() *WalletLedgerItem_GetID_Call {
	return &WalletLedgerItem_GetID_Call{Call: _e.mock.On("GetID")}
}

func (_c *WalletLedgerItem_GetID_Call) Run(run func()) *WalletLedgerItem_GetID_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *WalletLedgerItem_GetID_Call) Return(_a0 string) *WalletLedgerItem_GetID_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *WalletLedgerItem_GetID_Call) RunAndReturn(run func() string) *WalletLedgerItem_GetID_Call {
	_c.Call.Return(run)
	return _c
}

// GetMetadata provides a mock function with given fields:
func (_m *WalletLedgerItem) GetMetadata() map[string]interface{} {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetMetadata")
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

// WalletLedgerItem_GetMetadata_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetMetadata'
type WalletLedgerItem_GetMetadata_Call struct {
	*mock.Call
}

// GetMetadata is a helper method to define mock.On call
func (_e *WalletLedgerItem_Expecter) GetMetadata() *WalletLedgerItem_GetMetadata_Call {
	return &WalletLedgerItem_GetMetadata_Call{Call: _e.mock.On("GetMetadata")}
}

func (_c *WalletLedgerItem_GetMetadata_Call) Run(run func()) *WalletLedgerItem_GetMetadata_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *WalletLedgerItem_GetMetadata_Call) Return(_a0 map[string]interface{}) *WalletLedgerItem_GetMetadata_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *WalletLedgerItem_GetMetadata_Call) RunAndReturn(run func() map[string]interface{}) *WalletLedgerItem_GetMetadata_Call {
	_c.Call.Return(run)
	return _c
}

// GetUpdateTime provides a mock function with given fields:
func (_m *WalletLedgerItem) GetUpdateTime() int64 {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetUpdateTime")
	}

	var r0 int64
	if rf, ok := ret.Get(0).(func() int64); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int64)
	}

	return r0
}

// WalletLedgerItem_GetUpdateTime_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetUpdateTime'
type WalletLedgerItem_GetUpdateTime_Call struct {
	*mock.Call
}

// GetUpdateTime is a helper method to define mock.On call
func (_e *WalletLedgerItem_Expecter) GetUpdateTime() *WalletLedgerItem_GetUpdateTime_Call {
	return &WalletLedgerItem_GetUpdateTime_Call{Call: _e.mock.On("GetUpdateTime")}
}

func (_c *WalletLedgerItem_GetUpdateTime_Call) Run(run func()) *WalletLedgerItem_GetUpdateTime_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *WalletLedgerItem_GetUpdateTime_Call) Return(_a0 int64) *WalletLedgerItem_GetUpdateTime_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *WalletLedgerItem_GetUpdateTime_Call) RunAndReturn(run func() int64) *WalletLedgerItem_GetUpdateTime_Call {
	_c.Call.Return(run)
	return _c
}

// GetUserID provides a mock function with given fields:
func (_m *WalletLedgerItem) GetUserID() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetUserID")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// WalletLedgerItem_GetUserID_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetUserID'
type WalletLedgerItem_GetUserID_Call struct {
	*mock.Call
}

// GetUserID is a helper method to define mock.On call
func (_e *WalletLedgerItem_Expecter) GetUserID() *WalletLedgerItem_GetUserID_Call {
	return &WalletLedgerItem_GetUserID_Call{Call: _e.mock.On("GetUserID")}
}

func (_c *WalletLedgerItem_GetUserID_Call) Run(run func()) *WalletLedgerItem_GetUserID_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *WalletLedgerItem_GetUserID_Call) Return(_a0 string) *WalletLedgerItem_GetUserID_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *WalletLedgerItem_GetUserID_Call) RunAndReturn(run func() string) *WalletLedgerItem_GetUserID_Call {
	_c.Call.Return(run)
	return _c
}

// NewWalletLedgerItem creates a new instance of WalletLedgerItem. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewWalletLedgerItem(t interface {
	mock.TestingT
	Cleanup(func())
}) *WalletLedgerItem {
	mock := &WalletLedgerItem{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
