// Code generated by mockery. DO NOT EDIT.

package mockmetastorev1

import (
	context "context"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	mock "github.com/stretchr/testify/mock"
)

// MockMetastoreServiceServer is an autogenerated mock type for the MetastoreServiceServer type
type MockMetastoreServiceServer struct {
	mock.Mock
}

type MockMetastoreServiceServer_Expecter struct {
	mock *mock.Mock
}

func (_m *MockMetastoreServiceServer) EXPECT() *MockMetastoreServiceServer_Expecter {
	return &MockMetastoreServiceServer_Expecter{mock: &_m.Mock}
}

// AddBlock provides a mock function with given fields: _a0, _a1
func (_m *MockMetastoreServiceServer) AddBlock(_a0 context.Context, _a1 *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	ret := _m.Called(_a0, _a1)

	if len(ret) == 0 {
		panic("no return value specified for AddBlock")
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

// MockMetastoreServiceServer_AddBlock_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'AddBlock'
type MockMetastoreServiceServer_AddBlock_Call struct {
	*mock.Call
}

// AddBlock is a helper method to define mock.On call
//   - _a0 context.Context
//   - _a1 *metastorev1.AddBlockRequest
func (_e *MockMetastoreServiceServer_Expecter) AddBlock(_a0 interface{}, _a1 interface{}) *MockMetastoreServiceServer_AddBlock_Call {
	return &MockMetastoreServiceServer_AddBlock_Call{Call: _e.mock.On("AddBlock", _a0, _a1)}
}

func (_c *MockMetastoreServiceServer_AddBlock_Call) Run(run func(_a0 context.Context, _a1 *metastorev1.AddBlockRequest)) *MockMetastoreServiceServer_AddBlock_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*metastorev1.AddBlockRequest))
	})
	return _c
}

func (_c *MockMetastoreServiceServer_AddBlock_Call) Return(_a0 *metastorev1.AddBlockResponse, _a1 error) *MockMetastoreServiceServer_AddBlock_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockMetastoreServiceServer_AddBlock_Call) RunAndReturn(run func(context.Context, *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error)) *MockMetastoreServiceServer_AddBlock_Call {
	_c.Call.Return(run)
	return _c
}

// QueryMetadata provides a mock function with given fields: _a0, _a1
func (_m *MockMetastoreServiceServer) QueryMetadata(_a0 context.Context, _a1 *metastorev1.QueryMetadataRequest) (*metastorev1.QueryMetadataResponse, error) {
	ret := _m.Called(_a0, _a1)

	if len(ret) == 0 {
		panic("no return value specified for QueryMetadata")
	}

	var r0 *metastorev1.QueryMetadataResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *metastorev1.QueryMetadataRequest) (*metastorev1.QueryMetadataResponse, error)); ok {
		return rf(_a0, _a1)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *metastorev1.QueryMetadataRequest) *metastorev1.QueryMetadataResponse); ok {
		r0 = rf(_a0, _a1)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*metastorev1.QueryMetadataResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *metastorev1.QueryMetadataRequest) error); ok {
		r1 = rf(_a0, _a1)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockMetastoreServiceServer_QueryMetadata_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'QueryMetadata'
type MockMetastoreServiceServer_QueryMetadata_Call struct {
	*mock.Call
}

// QueryMetadata is a helper method to define mock.On call
//   - _a0 context.Context
//   - _a1 *metastorev1.QueryMetadataRequest
func (_e *MockMetastoreServiceServer_Expecter) QueryMetadata(_a0 interface{}, _a1 interface{}) *MockMetastoreServiceServer_QueryMetadata_Call {
	return &MockMetastoreServiceServer_QueryMetadata_Call{Call: _e.mock.On("QueryMetadata", _a0, _a1)}
}

func (_c *MockMetastoreServiceServer_QueryMetadata_Call) Run(run func(_a0 context.Context, _a1 *metastorev1.QueryMetadataRequest)) *MockMetastoreServiceServer_QueryMetadata_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*metastorev1.QueryMetadataRequest))
	})
	return _c
}

func (_c *MockMetastoreServiceServer_QueryMetadata_Call) Return(_a0 *metastorev1.QueryMetadataResponse, _a1 error) *MockMetastoreServiceServer_QueryMetadata_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockMetastoreServiceServer_QueryMetadata_Call) RunAndReturn(run func(context.Context, *metastorev1.QueryMetadataRequest) (*metastorev1.QueryMetadataResponse, error)) *MockMetastoreServiceServer_QueryMetadata_Call {
	_c.Call.Return(run)
	return _c
}

// ReadIndex provides a mock function with given fields: _a0, _a1
func (_m *MockMetastoreServiceServer) ReadIndex(_a0 context.Context, _a1 *metastorev1.ReadIndexRequest) (*metastorev1.ReadIndexResponse, error) {
	ret := _m.Called(_a0, _a1)

	if len(ret) == 0 {
		panic("no return value specified for ReadIndex")
	}

	var r0 *metastorev1.ReadIndexResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *metastorev1.ReadIndexRequest) (*metastorev1.ReadIndexResponse, error)); ok {
		return rf(_a0, _a1)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *metastorev1.ReadIndexRequest) *metastorev1.ReadIndexResponse); ok {
		r0 = rf(_a0, _a1)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*metastorev1.ReadIndexResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *metastorev1.ReadIndexRequest) error); ok {
		r1 = rf(_a0, _a1)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockMetastoreServiceServer_ReadIndex_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ReadIndex'
type MockMetastoreServiceServer_ReadIndex_Call struct {
	*mock.Call
}

// ReadIndex is a helper method to define mock.On call
//   - _a0 context.Context
//   - _a1 *metastorev1.ReadIndexRequest
func (_e *MockMetastoreServiceServer_Expecter) ReadIndex(_a0 interface{}, _a1 interface{}) *MockMetastoreServiceServer_ReadIndex_Call {
	return &MockMetastoreServiceServer_ReadIndex_Call{Call: _e.mock.On("ReadIndex", _a0, _a1)}
}

func (_c *MockMetastoreServiceServer_ReadIndex_Call) Run(run func(_a0 context.Context, _a1 *metastorev1.ReadIndexRequest)) *MockMetastoreServiceServer_ReadIndex_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*metastorev1.ReadIndexRequest))
	})
	return _c
}

func (_c *MockMetastoreServiceServer_ReadIndex_Call) Return(_a0 *metastorev1.ReadIndexResponse, _a1 error) *MockMetastoreServiceServer_ReadIndex_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockMetastoreServiceServer_ReadIndex_Call) RunAndReturn(run func(context.Context, *metastorev1.ReadIndexRequest) (*metastorev1.ReadIndexResponse, error)) *MockMetastoreServiceServer_ReadIndex_Call {
	_c.Call.Return(run)
	return _c
}

// mustEmbedUnimplementedMetastoreServiceServer provides a mock function with given fields:
func (_m *MockMetastoreServiceServer) mustEmbedUnimplementedMetastoreServiceServer() {
	_m.Called()
}

// MockMetastoreServiceServer_mustEmbedUnimplementedMetastoreServiceServer_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'mustEmbedUnimplementedMetastoreServiceServer'
type MockMetastoreServiceServer_mustEmbedUnimplementedMetastoreServiceServer_Call struct {
	*mock.Call
}

// mustEmbedUnimplementedMetastoreServiceServer is a helper method to define mock.On call
func (_e *MockMetastoreServiceServer_Expecter) mustEmbedUnimplementedMetastoreServiceServer() *MockMetastoreServiceServer_mustEmbedUnimplementedMetastoreServiceServer_Call {
	return &MockMetastoreServiceServer_mustEmbedUnimplementedMetastoreServiceServer_Call{Call: _e.mock.On("mustEmbedUnimplementedMetastoreServiceServer")}
}

func (_c *MockMetastoreServiceServer_mustEmbedUnimplementedMetastoreServiceServer_Call) Run(run func()) *MockMetastoreServiceServer_mustEmbedUnimplementedMetastoreServiceServer_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockMetastoreServiceServer_mustEmbedUnimplementedMetastoreServiceServer_Call) Return() *MockMetastoreServiceServer_mustEmbedUnimplementedMetastoreServiceServer_Call {
	_c.Call.Return()
	return _c
}

func (_c *MockMetastoreServiceServer_mustEmbedUnimplementedMetastoreServiceServer_Call) RunAndReturn(run func()) *MockMetastoreServiceServer_mustEmbedUnimplementedMetastoreServiceServer_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockMetastoreServiceServer creates a new instance of MockMetastoreServiceServer. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockMetastoreServiceServer(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockMetastoreServiceServer {
	mock := &MockMetastoreServiceServer{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}