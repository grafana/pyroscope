// SPDX-License-Identifier: AGPL-3.0-only

package bucket

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/thanos-io/objstore"
)

func TestPrefixedBucketClient(t *testing.T) {
	const prefix = "p"
	mockBucket := &ClientMock{}
	client := NewPrefixedBucketClient(mockBucket, prefix)

	t.Run("Upload", func(t *testing.T) {
		mockBucket.MockUpload(prefix+"/file", nil)
		err := client.Upload(context.Background(), "file", bytes.NewReader([]byte("123")))
		assert.NoError(t, err)
		mockBucket.AssertExpectations(t)
	})

	t.Run("Delete", func(t *testing.T) {
		mockBucket.MockDelete(prefix+"/file", nil)
		err := client.Delete(context.Background(), "file")
		assert.NoError(t, err)
		mockBucket.AssertExpectations(t)
	})

	t.Run("Iter", func(t *testing.T) {
		mockBucket.MockIter(prefix+"/dir", []string{"1"}, nil)
		err := client.Iter(context.Background(), "dir", func(s string) error {
			assert.Equal(t, "1", s)
			return nil
		})
		assert.NoError(t, err)
		mockBucket.AssertExpectations(t)
	})

	t.Run("Get", func(t *testing.T) {
		mockBucket.On("Get", mock.Anything, prefix+"/file").Return(io.NopCloser(bytes.NewReader([]byte(("1")))), nil)

		reader, err := client.Get(context.Background(), "file")
		assert.NoError(t, err)
		actualContent, err := io.ReadAll(reader)
		assert.NoError(t, err)
		assert.Equal(t, "1", string(actualContent))
		mockBucket.AssertExpectations(t)
	})

	t.Run("GetRange", func(t *testing.T) {
		mockBucket.On("GetRange", mock.Anything, prefix+"/file", mock.Anything, mock.Anything).Return(io.NopCloser(bytes.NewReader([]byte(("1")))), nil)

		reader, err := client.GetRange(context.Background(), "file", 0, 10)
		assert.NoError(t, err)
		actualContent, err := io.ReadAll(reader)
		assert.NoError(t, err)
		assert.Equal(t, "1", string(actualContent))
		mockBucket.AssertExpectations(t)
	})

	t.Run("Exists", func(t *testing.T) {
		mockBucket.MockExists(prefix+"/file", true, nil)
		exists, err := client.Exists(context.Background(), "file")
		assert.NoError(t, err)
		assert.True(t, exists)
		mockBucket.AssertExpectations(t)
	})

	t.Run("Attributes", func(t *testing.T) {
		mockBucket.On("Attributes", mock.Anything, prefix+"/file").Return(objstore.ObjectAttributes{}, nil)
		_, err := client.Attributes(context.Background(), "file")
		assert.NoError(t, err)
		mockBucket.AssertExpectations(t)
	})
}
