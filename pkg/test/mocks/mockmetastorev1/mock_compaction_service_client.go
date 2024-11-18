// Code generated by mockery. DO NOT EDIT.

package mockmetastorev1

import (
	context "context"

	grpc "google.golang.org/grpc"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"

	mock "github.com/stretchr/testify/mock"
)

// MockCompactionServiceClient is an autogenerated mock type for the CompactionServiceClient type
type MockCompactionServiceClient struct {
	mock.Mock
}

type MockCompactionServiceClient_Expecter struct {
	mock *mock.Mock
}

func (_m *MockCompactionServiceClient) EXPECT() *MockCompactionServiceClient_Expecter {
	return &MockCompactionServiceClient_Expecter{mock: &_m.Mock}
}

// PollCompactionJobs provides a mock function with given fields: ctx, in, opts
func (_m *MockCompactionServiceClient) PollCompactionJobs(ctx context.Context, in *metastorev1.PollCompactionJobsRequest, opts ...grpc.CallOption) (*metastorev1.PollCompactionJobsResponse, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for PollCompactionJobs")
	}

	var r0 *metastorev1.PollCompactionJobsResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *metastorev1.PollCompactionJobsRequest, ...grpc.CallOption) (*metastorev1.PollCompactionJobsResponse, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *metastorev1.PollCompactionJobsRequest, ...grpc.CallOption) *metastorev1.PollCompactionJobsResponse); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*metastorev1.PollCompactionJobsResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *metastorev1.PollCompactionJobsRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockCompactionServiceClient_PollCompactionJobs_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'PollCompactionJobs'
type MockCompactionServiceClient_PollCompactionJobs_Call struct {
	*mock.Call
}

// PollCompactionJobs is a helper method to define mock.On call
//   - ctx context.Context
//   - in *metastorev1.PollCompactionJobsRequest
//   - opts ...grpc.CallOption
func (_e *MockCompactionServiceClient_Expecter) PollCompactionJobs(ctx interface{}, in interface{}, opts ...interface{}) *MockCompactionServiceClient_PollCompactionJobs_Call {
	return &MockCompactionServiceClient_PollCompactionJobs_Call{Call: _e.mock.On("PollCompactionJobs",
		append([]interface{}{ctx, in}, opts...)...)}
}

func (_c *MockCompactionServiceClient_PollCompactionJobs_Call) Run(run func(ctx context.Context, in *metastorev1.PollCompactionJobsRequest, opts ...grpc.CallOption)) *MockCompactionServiceClient_PollCompactionJobs_Call {
	_c.Call.Run(func(args mock.Arguments) {
		variadicArgs := make([]grpc.CallOption, len(args)-2)
		for i, a := range args[2:] {
			if a != nil {
				variadicArgs[i] = a.(grpc.CallOption)
			}
		}
		run(args[0].(context.Context), args[1].(*metastorev1.PollCompactionJobsRequest), variadicArgs...)
	})
	return _c
}

func (_c *MockCompactionServiceClient_PollCompactionJobs_Call) Return(_a0 *metastorev1.PollCompactionJobsResponse, _a1 error) *MockCompactionServiceClient_PollCompactionJobs_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockCompactionServiceClient_PollCompactionJobs_Call) RunAndReturn(run func(context.Context, *metastorev1.PollCompactionJobsRequest, ...grpc.CallOption) (*metastorev1.PollCompactionJobsResponse, error)) *MockCompactionServiceClient_PollCompactionJobs_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockCompactionServiceClient creates a new instance of MockCompactionServiceClient. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockCompactionServiceClient(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockCompactionServiceClient {
	mock := &MockCompactionServiceClient{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}