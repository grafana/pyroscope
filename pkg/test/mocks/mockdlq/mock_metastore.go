// Code generated by mockery. DO NOT EDIT.

package mockdlq

import (
	context "context"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"

	mock "github.com/stretchr/testify/mock"
)

// MockMetastore is an autogenerated mock type for the Metastore type
type MockMetastore struct {
	mock.Mock
}

type MockMetastore_Expecter struct {
	mock *mock.Mock
}

func (_m *MockMetastore) EXPECT() *MockMetastore_Expecter {
	return &MockMetastore_Expecter{mock: &_m.Mock}
}

// AddRecoveredBlock provides a mock function with given fields: _a0, _a1
func (_m *MockMetastore) AddRecoveredBlock(_a0 context.Context, _a1 *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	ret := _m.Called(_a0, _a1)

	if len(ret) == 0 {
		panic("no return value specified for AddRecoveredBlock")
	}

	var r0 *metastorev1.AddBlockResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error)); ok {
		return rf(_a0, _a1)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *metastorev1.AddBlockRequest) *metastorev1.AddBlockResponse); ok {
		r0 = rf(_a0, _a1)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*metastorev1.AddBlockResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *metastorev1.AddBlockRequest) error); ok {
		r1 = rf(_a0, _a1)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockMetastore_AddRecoveredBlock_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'AddRecoveredBlock'
type MockMetastore_AddRecoveredBlock_Call struct {
	*mock.Call
}

// AddRecoveredBlock is a helper method to define mock.On call
//   - _a0 context.Context
//   - _a1 *metastorev1.AddBlockRequest
func (_e *MockMetastore_Expecter) AddRecoveredBlock(_a0 interface{}, _a1 interface{}) *MockMetastore_AddRecoveredBlock_Call {
	return &MockMetastore_AddRecoveredBlock_Call{Call: _e.mock.On("AddRecoveredBlock", _a0, _a1)}
}

func (_c *MockMetastore_AddRecoveredBlock_Call) Run(run func(_a0 context.Context, _a1 *metastorev1.AddBlockRequest)) *MockMetastore_AddRecoveredBlock_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*metastorev1.AddBlockRequest))
	})
	return _c
}

func (_c *MockMetastore_AddRecoveredBlock_Call) Return(_a0 *metastorev1.AddBlockResponse, _a1 error) *MockMetastore_AddRecoveredBlock_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockMetastore_AddRecoveredBlock_Call) RunAndReturn(run func(context.Context, *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error)) *MockMetastore_AddRecoveredBlock_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockMetastore creates a new instance of MockMetastore. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockMetastore(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockMetastore {
	mock := &MockMetastore{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
