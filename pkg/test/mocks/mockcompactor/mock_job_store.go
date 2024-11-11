// Code generated by mockery. DO NOT EDIT.

package mockcompactor

import (
	bbolt "go.etcd.io/bbolt"

	iter "github.com/grafana/pyroscope/pkg/iter"

	mock "github.com/stretchr/testify/mock"

	raft_log "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

// MockJobStore is an autogenerated mock type for the JobStore type
type MockJobStore struct {
	mock.Mock
}

type MockJobStore_Expecter struct {
	mock *mock.Mock
}

func (_m *MockJobStore) EXPECT() *MockJobStore_Expecter {
	return &MockJobStore_Expecter{mock: &_m.Mock}
}

// DeleteJobPlan provides a mock function with given fields: tx, name
func (_m *MockJobStore) DeleteJobPlan(tx *bbolt.Tx, name string) error {
	ret := _m.Called(tx, name)

	if len(ret) == 0 {
		panic("no return value specified for DeleteJobPlan")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(*bbolt.Tx, string) error); ok {
		r0 = rf(tx, name)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockJobStore_DeleteJobPlan_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'DeleteJobPlan'
type MockJobStore_DeleteJobPlan_Call struct {
	*mock.Call
}

// DeleteJobPlan is a helper method to define mock.On call
//   - tx *bbolt.Tx
//   - name string
func (_e *MockJobStore_Expecter) DeleteJobPlan(tx interface{}, name interface{}) *MockJobStore_DeleteJobPlan_Call {
	return &MockJobStore_DeleteJobPlan_Call{Call: _e.mock.On("DeleteJobPlan", tx, name)}
}

func (_c *MockJobStore_DeleteJobPlan_Call) Run(run func(tx *bbolt.Tx, name string)) *MockJobStore_DeleteJobPlan_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*bbolt.Tx), args[1].(string))
	})
	return _c
}

func (_c *MockJobStore_DeleteJobPlan_Call) Return(_a0 error) *MockJobStore_DeleteJobPlan_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockJobStore_DeleteJobPlan_Call) RunAndReturn(run func(*bbolt.Tx, string) error) *MockJobStore_DeleteJobPlan_Call {
	_c.Call.Return(run)
	return _c
}

// DeleteJobState provides a mock function with given fields: tx, name
func (_m *MockJobStore) DeleteJobState(tx *bbolt.Tx, name string) error {
	ret := _m.Called(tx, name)

	if len(ret) == 0 {
		panic("no return value specified for DeleteJobState")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(*bbolt.Tx, string) error); ok {
		r0 = rf(tx, name)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockJobStore_DeleteJobState_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'DeleteJobState'
type MockJobStore_DeleteJobState_Call struct {
	*mock.Call
}

// DeleteJobState is a helper method to define mock.On call
//   - tx *bbolt.Tx
//   - name string
func (_e *MockJobStore_Expecter) DeleteJobState(tx interface{}, name interface{}) *MockJobStore_DeleteJobState_Call {
	return &MockJobStore_DeleteJobState_Call{Call: _e.mock.On("DeleteJobState", tx, name)}
}

func (_c *MockJobStore_DeleteJobState_Call) Run(run func(tx *bbolt.Tx, name string)) *MockJobStore_DeleteJobState_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*bbolt.Tx), args[1].(string))
	})
	return _c
}

func (_c *MockJobStore_DeleteJobState_Call) Return(_a0 error) *MockJobStore_DeleteJobState_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockJobStore_DeleteJobState_Call) RunAndReturn(run func(*bbolt.Tx, string) error) *MockJobStore_DeleteJobState_Call {
	_c.Call.Return(run)
	return _c
}

// GetJobPlan provides a mock function with given fields: tx, name
func (_m *MockJobStore) GetJobPlan(tx *bbolt.Tx, name string) (*raft_log.CompactionJobPlan, error) {
	ret := _m.Called(tx, name)

	if len(ret) == 0 {
		panic("no return value specified for GetJobPlan")
	}

	var r0 *raft_log.CompactionJobPlan
	var r1 error
	if rf, ok := ret.Get(0).(func(*bbolt.Tx, string) (*raft_log.CompactionJobPlan, error)); ok {
		return rf(tx, name)
	}
	if rf, ok := ret.Get(0).(func(*bbolt.Tx, string) *raft_log.CompactionJobPlan); ok {
		r0 = rf(tx, name)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*raft_log.CompactionJobPlan)
		}
	}

	if rf, ok := ret.Get(1).(func(*bbolt.Tx, string) error); ok {
		r1 = rf(tx, name)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockJobStore_GetJobPlan_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetJobPlan'
type MockJobStore_GetJobPlan_Call struct {
	*mock.Call
}

// GetJobPlan is a helper method to define mock.On call
//   - tx *bbolt.Tx
//   - name string
func (_e *MockJobStore_Expecter) GetJobPlan(tx interface{}, name interface{}) *MockJobStore_GetJobPlan_Call {
	return &MockJobStore_GetJobPlan_Call{Call: _e.mock.On("GetJobPlan", tx, name)}
}

func (_c *MockJobStore_GetJobPlan_Call) Run(run func(tx *bbolt.Tx, name string)) *MockJobStore_GetJobPlan_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*bbolt.Tx), args[1].(string))
	})
	return _c
}

func (_c *MockJobStore_GetJobPlan_Call) Return(_a0 *raft_log.CompactionJobPlan, _a1 error) *MockJobStore_GetJobPlan_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockJobStore_GetJobPlan_Call) RunAndReturn(run func(*bbolt.Tx, string) (*raft_log.CompactionJobPlan, error)) *MockJobStore_GetJobPlan_Call {
	_c.Call.Return(run)
	return _c
}

// GetJobState provides a mock function with given fields: tx, name
func (_m *MockJobStore) GetJobState(tx *bbolt.Tx, name string) (*raft_log.CompactionJobState, error) {
	ret := _m.Called(tx, name)

	if len(ret) == 0 {
		panic("no return value specified for GetJobState")
	}

	var r0 *raft_log.CompactionJobState
	var r1 error
	if rf, ok := ret.Get(0).(func(*bbolt.Tx, string) (*raft_log.CompactionJobState, error)); ok {
		return rf(tx, name)
	}
	if rf, ok := ret.Get(0).(func(*bbolt.Tx, string) *raft_log.CompactionJobState); ok {
		r0 = rf(tx, name)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*raft_log.CompactionJobState)
		}
	}

	if rf, ok := ret.Get(1).(func(*bbolt.Tx, string) error); ok {
		r1 = rf(tx, name)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockJobStore_GetJobState_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetJobState'
type MockJobStore_GetJobState_Call struct {
	*mock.Call
}

// GetJobState is a helper method to define mock.On call
//   - tx *bbolt.Tx
//   - name string
func (_e *MockJobStore_Expecter) GetJobState(tx interface{}, name interface{}) *MockJobStore_GetJobState_Call {
	return &MockJobStore_GetJobState_Call{Call: _e.mock.On("GetJobState", tx, name)}
}

func (_c *MockJobStore_GetJobState_Call) Run(run func(tx *bbolt.Tx, name string)) *MockJobStore_GetJobState_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*bbolt.Tx), args[1].(string))
	})
	return _c
}

func (_c *MockJobStore_GetJobState_Call) Return(_a0 *raft_log.CompactionJobState, _a1 error) *MockJobStore_GetJobState_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockJobStore_GetJobState_Call) RunAndReturn(run func(*bbolt.Tx, string) (*raft_log.CompactionJobState, error)) *MockJobStore_GetJobState_Call {
	_c.Call.Return(run)
	return _c
}

// ListEntries provides a mock function with given fields: _a0
func (_m *MockJobStore) ListEntries(_a0 *bbolt.Tx) iter.Iterator[*raft_log.CompactionJobState] {
	ret := _m.Called(_a0)

	if len(ret) == 0 {
		panic("no return value specified for ListEntries")
	}

	var r0 iter.Iterator[*raft_log.CompactionJobState]
	if rf, ok := ret.Get(0).(func(*bbolt.Tx) iter.Iterator[*raft_log.CompactionJobState]); ok {
		r0 = rf(_a0)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(iter.Iterator[*raft_log.CompactionJobState])
		}
	}

	return r0
}

// MockJobStore_ListEntries_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ListEntries'
type MockJobStore_ListEntries_Call struct {
	*mock.Call
}

// ListEntries is a helper method to define mock.On call
//   - _a0 *bbolt.Tx
func (_e *MockJobStore_Expecter) ListEntries(_a0 interface{}) *MockJobStore_ListEntries_Call {
	return &MockJobStore_ListEntries_Call{Call: _e.mock.On("ListEntries", _a0)}
}

func (_c *MockJobStore_ListEntries_Call) Run(run func(_a0 *bbolt.Tx)) *MockJobStore_ListEntries_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*bbolt.Tx))
	})
	return _c
}

func (_c *MockJobStore_ListEntries_Call) Return(_a0 iter.Iterator[*raft_log.CompactionJobState]) *MockJobStore_ListEntries_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockJobStore_ListEntries_Call) RunAndReturn(run func(*bbolt.Tx) iter.Iterator[*raft_log.CompactionJobState]) *MockJobStore_ListEntries_Call {
	_c.Call.Return(run)
	return _c
}

// StoreJobPlan provides a mock function with given fields: _a0, _a1
func (_m *MockJobStore) StoreJobPlan(_a0 *bbolt.Tx, _a1 *raft_log.CompactionJobPlan) error {
	ret := _m.Called(_a0, _a1)

	if len(ret) == 0 {
		panic("no return value specified for StoreJobPlan")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(*bbolt.Tx, *raft_log.CompactionJobPlan) error); ok {
		r0 = rf(_a0, _a1)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockJobStore_StoreJobPlan_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'StoreJobPlan'
type MockJobStore_StoreJobPlan_Call struct {
	*mock.Call
}

// StoreJobPlan is a helper method to define mock.On call
//   - _a0 *bbolt.Tx
//   - _a1 *raft_log.CompactionJobPlan
func (_e *MockJobStore_Expecter) StoreJobPlan(_a0 interface{}, _a1 interface{}) *MockJobStore_StoreJobPlan_Call {
	return &MockJobStore_StoreJobPlan_Call{Call: _e.mock.On("StoreJobPlan", _a0, _a1)}
}

func (_c *MockJobStore_StoreJobPlan_Call) Run(run func(_a0 *bbolt.Tx, _a1 *raft_log.CompactionJobPlan)) *MockJobStore_StoreJobPlan_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*bbolt.Tx), args[1].(*raft_log.CompactionJobPlan))
	})
	return _c
}

func (_c *MockJobStore_StoreJobPlan_Call) Return(_a0 error) *MockJobStore_StoreJobPlan_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockJobStore_StoreJobPlan_Call) RunAndReturn(run func(*bbolt.Tx, *raft_log.CompactionJobPlan) error) *MockJobStore_StoreJobPlan_Call {
	_c.Call.Return(run)
	return _c
}

// UpdateJobState provides a mock function with given fields: _a0, _a1
func (_m *MockJobStore) UpdateJobState(_a0 *bbolt.Tx, _a1 *raft_log.CompactionJobState) error {
	ret := _m.Called(_a0, _a1)

	if len(ret) == 0 {
		panic("no return value specified for UpdateJobState")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(*bbolt.Tx, *raft_log.CompactionJobState) error); ok {
		r0 = rf(_a0, _a1)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockJobStore_UpdateJobState_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'UpdateJobState'
type MockJobStore_UpdateJobState_Call struct {
	*mock.Call
}

// UpdateJobState is a helper method to define mock.On call
//   - _a0 *bbolt.Tx
//   - _a1 *raft_log.CompactionJobState
func (_e *MockJobStore_Expecter) UpdateJobState(_a0 interface{}, _a1 interface{}) *MockJobStore_UpdateJobState_Call {
	return &MockJobStore_UpdateJobState_Call{Call: _e.mock.On("UpdateJobState", _a0, _a1)}
}

func (_c *MockJobStore_UpdateJobState_Call) Run(run func(_a0 *bbolt.Tx, _a1 *raft_log.CompactionJobState)) *MockJobStore_UpdateJobState_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*bbolt.Tx), args[1].(*raft_log.CompactionJobState))
	})
	return _c
}

func (_c *MockJobStore_UpdateJobState_Call) Return(_a0 error) *MockJobStore_UpdateJobState_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockJobStore_UpdateJobState_Call) RunAndReturn(run func(*bbolt.Tx, *raft_log.CompactionJobState) error) *MockJobStore_UpdateJobState_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockJobStore creates a new instance of MockJobStore. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockJobStore(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockJobStore {
	mock := &MockJobStore{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}