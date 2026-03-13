package reader

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func chunksFunc(chunks [][]byte) func() ([]byte, error) {
	i := 0
	return func() ([]byte, error) {
		if i >= len(chunks) {
			return nil, io.EOF
		}
		chunk := chunks[i]
		i++
		return chunk, nil
	}
}

func TestUploadReader_Read(t *testing.T) {
	t.Parallel()

	t.Run("single chunk", func(t *testing.T) {
		t.Parallel()
		r := New(context.Background(), chunksFunc([][]byte{[]byte("hello")}), 0)
		data, err := io.ReadAll(r)
		require.NoError(t, err)
		assert.Equal(t, "hello", string(data))
	})

	t.Run("multiple chunks", func(t *testing.T) {
		t.Parallel()
		r := New(context.Background(), chunksFunc([][]byte{
			[]byte("abc"),
			[]byte("def"),
			[]byte("ghi"),
		}), 0)
		data, err := io.ReadAll(r)
		require.NoError(t, err)
		assert.Equal(t, "abcdefghi", string(data))
	})

	t.Run("empty stream", func(t *testing.T) {
		t.Parallel()
		r := New(context.Background(), chunksFunc(nil), 0)
		data, err := io.ReadAll(r)
		require.NoError(t, err)
		assert.Empty(t, data)
	})

	t.Run("small buffer", func(t *testing.T) {
		t.Parallel()
		r := New(context.Background(), chunksFunc([][]byte{[]byte("hello world")}), 0)
		buf := make([]byte, 3)
		var result []byte
		for {
			n, err := r.Read(buf)
			result = append(result, buf[:n]...)
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
		}
		assert.Equal(t, "hello world", string(result))
	})

	t.Run("nextFunc error on first call", func(t *testing.T) {
		t.Parallel()
		r := New(context.Background(), func() ([]byte, error) {
			return nil, fmt.Errorf("network error")
		}, 0)
		buf := make([]byte, 1024)
		_, err := r.Read(buf)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "network error")
	})

	t.Run("error after some chunks", func(t *testing.T) {
		t.Parallel()
		callCount := 0
		r := New(context.Background(), func() ([]byte, error) {
			callCount++
			if callCount == 1 {
				return []byte("abc"), nil
			}
			return nil, fmt.Errorf("broken")
		}, 0)

		// First read should succeed with data from the first chunk.
		buf := make([]byte, 1024)
		n, err := r.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, "abc", string(buf[:n]))

		// Next read should get an error when fetching the next chunk.
		_, err = r.Read(buf)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "broken")
	})
}

func TestUploadReader_Size(t *testing.T) {
	t.Parallel()

	t.Run("no data", func(t *testing.T) {
		t.Parallel()
		r := New(context.Background(), chunksFunc(nil), 0)
		_, _ = io.ReadAll(r)
		assert.Equal(t, uint64(0), r.Size())
	})

	t.Run("after reading all", func(t *testing.T) {
		t.Parallel()
		r := New(context.Background(), chunksFunc([][]byte{
			[]byte("hello"),
			[]byte("world"),
		}), 0)
		_, err := io.ReadAll(r)
		require.NoError(t, err)
		assert.Equal(t, uint64(10), r.Size())
	})

	t.Run("tracks incrementally", func(t *testing.T) {
		t.Parallel()
		r := New(context.Background(), chunksFunc([][]byte{
			[]byte("aaa"),
			[]byte("bbb"),
		}), 0)

		assert.Equal(t, uint64(0), r.Size())

		buf := make([]byte, 1024)
		n, err := r.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, 3, n)
		assert.Equal(t, uint64(3), r.Size())
	})
}

func TestUploadReader_MaxSize(t *testing.T) {
	t.Parallel()

	t.Run("within limit", func(t *testing.T) {
		t.Parallel()
		r := New(context.Background(), chunksFunc([][]byte{
			[]byte("abc"),
			[]byte("def"),
		}), 10)
		data, err := io.ReadAll(r)
		require.NoError(t, err)
		assert.Equal(t, "abcdef", string(data))
	})

	t.Run("exceeds limit", func(t *testing.T) {
		t.Parallel()
		r := New(context.Background(), chunksFunc([][]byte{
			[]byte("abc"),
			[]byte("def"),
			[]byte("ghi"),
		}), 5)
		_, err := io.ReadAll(r)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum allowed size")
	})

	t.Run("zero means unlimited", func(t *testing.T) {
		t.Parallel()
		r := New(context.Background(), chunksFunc([][]byte{
			[]byte("abc"),
			[]byte("def"),
		}), 0)
		data, err := io.ReadAll(r)
		require.NoError(t, err)
		assert.Equal(t, "abcdef", string(data))
	})
}

func TestUploadReader_ContextCancellation(t *testing.T) {
	t.Parallel()

	t.Run("cancelled before first read", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		r := New(ctx, func() ([]byte, error) {
			t.Fatal("nextFunc should not be called when context is cancelled")
			return nil, nil
		}, 0)

		buf := make([]byte, 1024)
		_, err := r.Read(buf)
		require.Error(t, err)
	})

	t.Run("cancelled between chunks", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())

		callCount := 0
		r := New(ctx, func() ([]byte, error) {
			callCount++
			if callCount == 1 {
				return []byte("first"), nil
			}
			// Cancel context before returning the second chunk.
			cancel()
			return []byte("second"), nil
		}, 0)

		// Read the first chunk.
		buf := make([]byte, 1024)
		n, err := r.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, "first", string(buf[:n]))

		// The second chunk was returned but context was cancelled.
		// The reader should detect the cancelled context on the next next() call.
		// Read the second chunk (already fetched by internal logic).
		n, err = r.Read(buf)
		if err != nil {
			// Context cancellation detected
			return
		}
		assert.Equal(t, "second", string(buf[:n]))

		// Now the third call to next() should detect cancelled context.
		_, err = r.Read(buf)
		require.Error(t, err)
	})
}

func TestUploadReader_ReadAll(t *testing.T) {
	t.Parallel()

	r := New(context.Background(), chunksFunc([][]byte{
		[]byte("The quick "),
		[]byte("brown fox "),
		[]byte("jumps over the lazy dog"),
	}), 0)

	data, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, "The quick brown fox jumps over the lazy dog", string(data))
	assert.Equal(t, uint64(len("The quick brown fox jumps over the lazy dog")), r.Size())
}
