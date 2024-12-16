// Code generated by mockery v2.50.0. DO NOT EDIT.

package mocks

import (
	runtime "github.com/heroiclabs/nakama-common/runtime"
	mock "github.com/stretchr/testify/mock"
)

// MockIAPConfig is an autogenerated mock type for the IAPConfig type
type MockIAPConfig struct {
	mock.Mock
}

type MockIAPConfig_Expecter struct {
	mock *mock.Mock
}

func (_m *MockIAPConfig) EXPECT() *MockIAPConfig_Expecter {
	return &MockIAPConfig_Expecter{mock: &_m.Mock}
}

// GetApple provides a mock function with no fields
func (_m *MockIAPConfig) GetApple() runtime.IAPAppleConfig {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetApple")
	}

	var r0 runtime.IAPAppleConfig
	if rf, ok := ret.Get(0).(func() runtime.IAPAppleConfig); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(runtime.IAPAppleConfig)
		}
	}

	return r0
}

// MockIAPConfig_GetApple_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetApple'
type MockIAPConfig_GetApple_Call struct {
	*mock.Call
}

// GetApple is a helper method to define mock.On call
func (_e *MockIAPConfig_Expecter) GetApple() *MockIAPConfig_GetApple_Call {
	return &MockIAPConfig_GetApple_Call{Call: _e.mock.On("GetApple")}
}

func (_c *MockIAPConfig_GetApple_Call) Run(run func()) *MockIAPConfig_GetApple_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockIAPConfig_GetApple_Call) Return(_a0 runtime.IAPAppleConfig) *MockIAPConfig_GetApple_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockIAPConfig_GetApple_Call) RunAndReturn(run func() runtime.IAPAppleConfig) *MockIAPConfig_GetApple_Call {
	_c.Call.Return(run)
	return _c
}

// GetFacebookInstant provides a mock function with no fields
func (_m *MockIAPConfig) GetFacebookInstant() runtime.IAPFacebookInstantConfig {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetFacebookInstant")
	}

	var r0 runtime.IAPFacebookInstantConfig
	if rf, ok := ret.Get(0).(func() runtime.IAPFacebookInstantConfig); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(runtime.IAPFacebookInstantConfig)
		}
	}

	return r0
}

// MockIAPConfig_GetFacebookInstant_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetFacebookInstant'
type MockIAPConfig_GetFacebookInstant_Call struct {
	*mock.Call
}

// GetFacebookInstant is a helper method to define mock.On call
func (_e *MockIAPConfig_Expecter) GetFacebookInstant() *MockIAPConfig_GetFacebookInstant_Call {
	return &MockIAPConfig_GetFacebookInstant_Call{Call: _e.mock.On("GetFacebookInstant")}
}

func (_c *MockIAPConfig_GetFacebookInstant_Call) Run(run func()) *MockIAPConfig_GetFacebookInstant_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockIAPConfig_GetFacebookInstant_Call) Return(_a0 runtime.IAPFacebookInstantConfig) *MockIAPConfig_GetFacebookInstant_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockIAPConfig_GetFacebookInstant_Call) RunAndReturn(run func() runtime.IAPFacebookInstantConfig) *MockIAPConfig_GetFacebookInstant_Call {
	_c.Call.Return(run)
	return _c
}

// GetGoogle provides a mock function with no fields
func (_m *MockIAPConfig) GetGoogle() runtime.IAPGoogleConfig {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetGoogle")
	}

	var r0 runtime.IAPGoogleConfig
	if rf, ok := ret.Get(0).(func() runtime.IAPGoogleConfig); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(runtime.IAPGoogleConfig)
		}
	}

	return r0
}

// MockIAPConfig_GetGoogle_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetGoogle'
type MockIAPConfig_GetGoogle_Call struct {
	*mock.Call
}

// GetGoogle is a helper method to define mock.On call
func (_e *MockIAPConfig_Expecter) GetGoogle() *MockIAPConfig_GetGoogle_Call {
	return &MockIAPConfig_GetGoogle_Call{Call: _e.mock.On("GetGoogle")}
}

func (_c *MockIAPConfig_GetGoogle_Call) Run(run func()) *MockIAPConfig_GetGoogle_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockIAPConfig_GetGoogle_Call) Return(_a0 runtime.IAPGoogleConfig) *MockIAPConfig_GetGoogle_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockIAPConfig_GetGoogle_Call) RunAndReturn(run func() runtime.IAPGoogleConfig) *MockIAPConfig_GetGoogle_Call {
	_c.Call.Return(run)
	return _c
}

// GetHuawei provides a mock function with no fields
func (_m *MockIAPConfig) GetHuawei() runtime.IAPHuaweiConfig {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetHuawei")
	}

	var r0 runtime.IAPHuaweiConfig
	if rf, ok := ret.Get(0).(func() runtime.IAPHuaweiConfig); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(runtime.IAPHuaweiConfig)
		}
	}

	return r0
}

// MockIAPConfig_GetHuawei_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetHuawei'
type MockIAPConfig_GetHuawei_Call struct {
	*mock.Call
}

// GetHuawei is a helper method to define mock.On call
func (_e *MockIAPConfig_Expecter) GetHuawei() *MockIAPConfig_GetHuawei_Call {
	return &MockIAPConfig_GetHuawei_Call{Call: _e.mock.On("GetHuawei")}
}

func (_c *MockIAPConfig_GetHuawei_Call) Run(run func()) *MockIAPConfig_GetHuawei_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockIAPConfig_GetHuawei_Call) Return(_a0 runtime.IAPHuaweiConfig) *MockIAPConfig_GetHuawei_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockIAPConfig_GetHuawei_Call) RunAndReturn(run func() runtime.IAPHuaweiConfig) *MockIAPConfig_GetHuawei_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockIAPConfig creates a new instance of MockIAPConfig. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockIAPConfig(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockIAPConfig {
	mock := &MockIAPConfig{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
