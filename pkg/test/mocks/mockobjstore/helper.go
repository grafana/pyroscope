package mockobjstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/thanos-io/objstore"
)

// ErrObjectDoesNotExist is used in tests to simulate objstore.Bucket.IsObjNotFoundErr().
var ErrObjectDoesNotExist = errors.New("object does not exist")

func NewMockBucketWithHelper(t testing.TB) *MockBucket {
	m := &MockBucket{}
	m.EXPECT().IsObjNotFoundErr(mock.Anything).RunAndReturn(func(err error) bool {
		return errors.Is(err, ErrObjectDoesNotExist)
	})
	m.EXPECT().Name().Return("mock")
	return m
}

func (m *MockBucket) MockIter(prefix string, objects []string, err error) {
	m.MockIterWithCallback(prefix, objects, err, nil)
}

// MockIterWithCallback is a convenient method to mock Iter() and get a callback called when the Iter
// API is called.
func (m *MockBucket) MockIterWithCallback(prefix string, objects []string, err error, cb func()) {
	// implement IsObjNotFoundErr
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

func (m *MockBucket) MockUpload(name string, err error) {
	m.On("Upload", mock.Anything, name, mock.Anything, mock.Anything).Return(err)
}

func (m *MockBucket) MockAttributes(name string, attrs objstore.ObjectAttributes, err error) {
	m.On("Attributes", mock.Anything, name).Return(attrs, err)
}

func (m *MockBucket) MockDelete(name string, err error) {
	m.On("Delete", mock.Anything, name).Return(err)
}

func (m *MockBucket) MockExists(name string, exists bool, err error) {
	m.On("Exists", mock.Anything, name).Return(exists, err)
}

// MockGet is a convenient method to mock Get() and Exists()
func (m *MockBucket) MockGet(name, content string, err error) {
	if content != "" {
		m.On("Exists", mock.Anything, name).Return(true, err)
		m.On("Attributes", mock.Anything, name).Return(objstore.ObjectAttributes{
			Size:         int64(len(content)),
			LastModified: time.Now(),
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
