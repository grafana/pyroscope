// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/bucket/client_mock.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package objstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/thanos-io/objstore"
)

// ErrObjectDoesNotExist is used in tests to simulate objstore.Bucket.IsObjNotFoundErr().
var ErrObjectDoesNotExist = errors.New("object does not exist")

// ClientMock mocks objstore.Bucket
type ClientMock struct {
	mock.Mock
}

// Upload mocks objstore.Bucket.Upload()
func (m *ClientMock) Upload(ctx context.Context, name string, r io.Reader) error {
	args := m.Called(ctx, name, r)
	return args.Error(0)
}

func (m *ClientMock) MockUpload(name string, err error) {
	m.On("Upload", mock.Anything, name, mock.Anything).Return(err)
}

// Delete mocks objstore.Bucket.Delete()
func (m *ClientMock) Delete(ctx context.Context, name string) error {
	args := m.Called(ctx, name)
	return args.Error(0)
}

// Name mocks objstore.Bucket.Name()
func (m *ClientMock) Name() string {
	return "mock"
}

// Iter mocks objstore.Bucket.Iter()
func (m *ClientMock) Iter(ctx context.Context, dir string, f func(string) error, options ...objstore.IterOption) error {
	args := m.Called(ctx, dir, f, options)
	return args.Error(0)
}

// MockIter is a convenient method to mock Iter()
func (m *ClientMock) MockIter(prefix string, objects []string, err error) {
	m.MockIterWithCallback(prefix, objects, err, nil)
}

// MockIterWithCallback is a convenient method to mock Iter() and get a callback called when the Iter
// API is called.
func (m *ClientMock) MockIterWithCallback(prefix string, objects []string, err error, cb func()) {
	m.On("Iter", mock.Anything, prefix, mock.Anything, mock.Anything).Return(err).Run(func(args mock.Arguments) {
		if cb != nil {
			cb()
		}

		f := args.Get(2).(func(string) error)

		for _, o := range objects {
			if f(o) != nil {
				break
			}
		}
	})
}

// Get mocks objstore.Bucket.Get()
func (m *ClientMock) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	args := m.Called(ctx, name)

	// Allow to mock the Get() with a function which is called each time.
	if fn, ok := args.Get(0).(func(ctx context.Context, name string) (io.ReadCloser, error)); ok {
		return fn(ctx, name)
	}

	val, err := args.Get(0), args.Error(1)
	if val == nil {
		return nil, err
	}
	return val.(io.ReadCloser), err
}

func (m *ClientMock) ReaderAt(ctx context.Context, name string) (ReaderAtCloser, error) {
	args := m.Called(ctx, name)

	// Allow to mock the ReaderAt() with a function which is called each time.
	if fn, ok := args.Get(0).(func(ctx context.Context, name string) (ReaderAtCloser, error)); ok {
		return fn(ctx, name)
	}

	val, err := args.Get(0), args.Error(1)
	if val == nil {
		return nil, err
	}
	return val.(ReaderAtCloser), err
}

// MockGet is a convenient method to mock Get() and Exists()
func (m *ClientMock) MockGet(name, content string, err error) {
	m.MockGetAndLastModified(name, content, time.Now(), err)
}

func (m *ClientMock) MockGetAndLastModified(name, content string, lastModified time.Time, err error) {
	if content != "" {
		m.On("Exists", mock.Anything, name).Return(true, err)
		m.On("Attributes", mock.Anything, name).Return(objstore.ObjectAttributes{
			Size:         int64(len(content)),
			LastModified: lastModified,
		}, nil)

		// Since we return an ReadCloser and it can be consumed only once,
		// each time the mocked Get() is called we do create a new one, so
		// that getting the same mocked object twice works as expected.
		m.On("Get", mock.Anything, name).Return(func(_ context.Context, _ string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader([]byte(content))), err
		})
	} else {
		m.On("Exists", mock.Anything, name).Return(false, err)
		m.On("Get", mock.Anything, name).Return(nil, ErrObjectDoesNotExist)
		m.On("Attributes", mock.Anything, name).Return(nil, ErrObjectDoesNotExist)
	}
}

func (m *ClientMock) MockAttributes(name string, attrs objstore.ObjectAttributes, err error) {
	m.On("Attributes", mock.Anything, name).Return(attrs, err)
}

func (m *ClientMock) MockDelete(name string, err error) {
	m.On("Delete", mock.Anything, name).Return(err)
}

func (m *ClientMock) MockExists(name string, exists bool, err error) {
	m.On("Exists", mock.Anything, name).Return(exists, err)
}

// GetRange mocks objstore.Bucket.GetRange()
func (m *ClientMock) GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error) {
	args := m.Called(ctx, name, off, length)
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

// Exists mocks objstore.Bucket.Exists()
func (m *ClientMock) Exists(ctx context.Context, name string) (bool, error) {
	args := m.Called(ctx, name)
	return args.Bool(0), args.Error(1)
}

// IsObjNotFoundErr mocks objstore.Bucket.IsObjNotFoundErr()
func (m *ClientMock) IsObjNotFoundErr(err error) bool {
	return errors.Is(err, ErrObjectDoesNotExist)
}

func (m *ClientMock) IsAccessDeniedErr(_ error) bool {
	return false
}

// ObjectSize mocks objstore.Bucket.Attributes()
func (m *ClientMock) Attributes(ctx context.Context, name string) (objstore.ObjectAttributes, error) {
	args := m.Called(ctx, name)
	return args.Get(0).(objstore.ObjectAttributes), args.Error(1)
}

// Close mocks objstore.Bucket.Close()
func (m *ClientMock) Close() error {
	return nil
}
