// Code generated by mockery v2.50.0. DO NOT EDIT.

package mocks

import mock "github.com/stretchr/testify/mock"

// MockGoogleAuthConfig is an autogenerated mock type for the GoogleAuthConfig type
type MockGoogleAuthConfig struct {
	mock.Mock
}

type MockGoogleAuthConfig_Expecter struct {
	mock *mock.Mock
}

func (_m *MockGoogleAuthConfig) EXPECT() *MockGoogleAuthConfig_Expecter {
	return &MockGoogleAuthConfig_Expecter{mock: &_m.Mock}
}

// GetCredentialsJSON provides a mock function with no fields
func (_m *MockGoogleAuthConfig) GetCredentialsJSON() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetCredentialsJSON")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// MockGoogleAuthConfig_GetCredentialsJSON_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetCredentialsJSON'
type MockGoogleAuthConfig_GetCredentialsJSON_Call struct {
	*mock.Call
}

// GetCredentialsJSON is a helper method to define mock.On call
func (_e *MockGoogleAuthConfig_Expecter) GetCredentialsJSON() *MockGoogleAuthConfig_GetCredentialsJSON_Call {
	return &MockGoogleAuthConfig_GetCredentialsJSON_Call{Call: _e.mock.On("GetCredentialsJSON")}
}

func (_c *MockGoogleAuthConfig_GetCredentialsJSON_Call) Run(run func()) *MockGoogleAuthConfig_GetCredentialsJSON_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockGoogleAuthConfig_GetCredentialsJSON_Call) Return(_a0 string) *MockGoogleAuthConfig_GetCredentialsJSON_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockGoogleAuthConfig_GetCredentialsJSON_Call) RunAndReturn(run func() string) *MockGoogleAuthConfig_GetCredentialsJSON_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockGoogleAuthConfig creates a new instance of MockGoogleAuthConfig. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockGoogleAuthConfig(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockGoogleAuthConfig {
	mock := &MockGoogleAuthConfig{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
