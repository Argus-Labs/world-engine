// Code generated by mockery v2.50.0. DO NOT EDIT.

package mocks

import mock "github.com/stretchr/testify/mock"

// MockSocialConfigSteam is an autogenerated mock type for the SocialConfigSteam type
type MockSocialConfigSteam struct {
	mock.Mock
}

type MockSocialConfigSteam_Expecter struct {
	mock *mock.Mock
}

func (_m *MockSocialConfigSteam) EXPECT() *MockSocialConfigSteam_Expecter {
	return &MockSocialConfigSteam_Expecter{mock: &_m.Mock}
}

// GetAppID provides a mock function with no fields
func (_m *MockSocialConfigSteam) GetAppID() int {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetAppID")
	}

	var r0 int
	if rf, ok := ret.Get(0).(func() int); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int)
	}

	return r0
}

// MockSocialConfigSteam_GetAppID_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetAppID'
type MockSocialConfigSteam_GetAppID_Call struct {
	*mock.Call
}

// GetAppID is a helper method to define mock.On call
func (_e *MockSocialConfigSteam_Expecter) GetAppID() *MockSocialConfigSteam_GetAppID_Call {
	return &MockSocialConfigSteam_GetAppID_Call{Call: _e.mock.On("GetAppID")}
}

func (_c *MockSocialConfigSteam_GetAppID_Call) Run(run func()) *MockSocialConfigSteam_GetAppID_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockSocialConfigSteam_GetAppID_Call) Return(_a0 int) *MockSocialConfigSteam_GetAppID_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockSocialConfigSteam_GetAppID_Call) RunAndReturn(run func() int) *MockSocialConfigSteam_GetAppID_Call {
	_c.Call.Return(run)
	return _c
}

// GetPublisherKey provides a mock function with no fields
func (_m *MockSocialConfigSteam) GetPublisherKey() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetPublisherKey")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// MockSocialConfigSteam_GetPublisherKey_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetPublisherKey'
type MockSocialConfigSteam_GetPublisherKey_Call struct {
	*mock.Call
}

// GetPublisherKey is a helper method to define mock.On call
func (_e *MockSocialConfigSteam_Expecter) GetPublisherKey() *MockSocialConfigSteam_GetPublisherKey_Call {
	return &MockSocialConfigSteam_GetPublisherKey_Call{Call: _e.mock.On("GetPublisherKey")}
}

func (_c *MockSocialConfigSteam_GetPublisherKey_Call) Run(run func()) *MockSocialConfigSteam_GetPublisherKey_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockSocialConfigSteam_GetPublisherKey_Call) Return(_a0 string) *MockSocialConfigSteam_GetPublisherKey_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockSocialConfigSteam_GetPublisherKey_Call) RunAndReturn(run func() string) *MockSocialConfigSteam_GetPublisherKey_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockSocialConfigSteam creates a new instance of MockSocialConfigSteam. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockSocialConfigSteam(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockSocialConfigSteam {
	mock := &MockSocialConfigSteam{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}