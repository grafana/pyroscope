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
		r := New(context.Background(), chunksFunc([][]byte{[]byte("hello")}))
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
		}))
		data, err := io.ReadAll(r)
		require.NoError(t, err)
		assert.Equal(t, "abcdefghi", string(data))
	})

	t.Run("empty stream", func(t *testing.T) {
		t.Parallel()
		r := New(context.Background(), chunksFunc(nil))
		data, err := io.ReadAll(r)
		require.NoError(t, err)
		assert.Empty(t, data)
	})

	t.Run("small buffer", func(t *testing.T) {
		t.Parallel()
		r := New(context.Background(), chunksFunc([][]byte{[]byte("hello world")}))
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
		})
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
		})

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
		r := New(context.Background(), chunksFunc(nil))
		_, _ = io.ReadAll(r)
		assert.Equal(t, uint64(0), r.Size())
	})

	t.Run("after reading all", func(t *testing.T) {
		t.Parallel()
		r := New(context.Background(), chunksFunc([][]byte{
			[]byte("hello"),
			[]byte("world"),
		}))
		_, err := io.ReadAll(r)
		require.NoError(t, err)
		assert.Equal(t, uint64(10), r.Size())
	})

	t.Run("tracks incrementally", func(t *testing.T) {
		t.Parallel()
		r := New(context.Background(), chunksFunc([][]byte{
			[]byte("aaa"),
			[]byte("bbb"),
		}))

		assert.Equal(t, uint64(0), r.Size())

		buf := make([]byte, 1024)
		n, err := r.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, 3, n)
		assert.Equal(t, uint64(3), r.Size())
	})
}

func TestMaxSizeReader(t *testing.T) {
	t.Parallel()

	t.Run("within limit", func(t *testing.T) {
		t.Parallel()
		r := New(context.Background(), chunksFunc([][]byte{
			[]byte("abc"),
			[]byte("def"),
		}))
		lr := NewMaxSizeReader(r, 10)
		data, err := io.ReadAll(lr)
		require.NoError(t, err)
		assert.Equal(t, "abcdef", string(data))
	})

	t.Run("exceeds limit", func(t *testing.T) {
		t.Parallel()
		r := New(context.Background(), chunksFunc([][]byte{
			[]byte("abc"),
			[]byte("def"),
			[]byte("ghi"),
		}))
		lr := NewMaxSizeReader(r, 5)
		_, err := io.ReadAll(lr)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum allowed size")
	})

	t.Run("returns already-read bytes when limit exceeded", func(t *testing.T) {
		t.Parallel()
		r := New(context.Background(), chunksFunc([][]byte{
			[]byte("abcdef"),
		}))
		lr := NewMaxSizeReader(r, 5)

		buf := make([]byte, 16)
		n, err := lr.Read(buf)
		require.Error(t, err)
		assert.Equal(t, 6, n)
		assert.Equal(t, "abcdef", string(buf[:n]))
		assert.Contains(t, err.Error(), "exceeds maximum allowed size")
	})

	t.Run("zero means unlimited", func(t *testing.T) {
		t.Parallel()
		r := New(context.Background(), chunksFunc([][]byte{
			[]byte("abc"),
			[]byte("def"),
		}))
		lr := NewMaxSizeReader(r, 0)
		data, err := io.ReadAll(lr)
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
		})

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
		})

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
	}))

	data, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, "The quick brown fox jumps over the lazy dog", string(data))
	assert.Equal(t, uint64(len("The quick brown fox jumps over the lazy dog")), r.Size())
}

// eofReader is a reader that returns data and io.EOF simultaneously,
// unlike bytes.Buffer which never does this.
type eofReader struct {
	data []byte
	read bool
}

func (r *eofReader) Read(p []byte) (int, error) {
	if r.read {
		return 0, io.EOF
	}
	r.read = true
	n := copy(p, r.data)
	return n, io.EOF
}

func TestUploadReader_SimultaneousDataAndEOF(t *testing.T) {
	t.Parallel()

	t.Run("does not drop bytes when reader returns data with EOF", func(t *testing.T) {
		t.Parallel()
		callCount := 0
		r := &UploadReader{
			context: context.Background(),
			nextFunc: func() ([]byte, error) {
				return nil, io.EOF
			},
			// Use an eofReader that returns data and EOF simultaneously.
			cur: &eofReader{data: []byte("hello")},
		}
		_ = callCount

		buf := make([]byte, 1024)
		n, err := r.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, "hello", string(buf[:n]))
		assert.Equal(t, uint64(5), r.Size())

		// Next read should return EOF.
		n, err = r.Read(buf)
		assert.Equal(t, 0, n)
		assert.Equal(t, io.EOF, err)
	})

	t.Run("data with EOF followed by more chunks", func(t *testing.T) {
		t.Parallel()
		chunkIdx := 0
		chunks := [][]byte{[]byte(" world")}
		r := &UploadReader{
			context: context.Background(),
			nextFunc: func() ([]byte, error) {
				if chunkIdx >= len(chunks) {
					return nil, io.EOF
				}
				chunk := chunks[chunkIdx]
				chunkIdx++
				return chunk, nil
			},
			cur: &eofReader{data: []byte("hello")},
		}

		data, err := io.ReadAll(r)
		require.NoError(t, err)
		assert.Equal(t, "hello world", string(data))
	})
}
