package index

import (
	"bytes"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPutBufferWriterToPool(t *testing.T) {
	// Isolate the pool; sync.Pool does not guarantee pointer reuse.
	pool = sync.Pool{New: func() interface{} { return NewBufferWriter() }}
	t.Cleanup(func() {
		pool = sync.Pool{New: func() interface{} { return NewBufferWriter() }}
	})

	for _, capacity := range []int{8192, maxPooledBufferCapacity, maxPooledBufferCapacity + 1} {
		fw := &BufferWriter{buf: bytes.NewBuffer(make([]byte, 0, capacity))}
		require.NoError(t, fw.Write([]byte("previous job")))
		PutBufferWriterToPool(fw)

		got := GetBufferWriterFromPool()
		require.Zero(t, got.Pos())
		require.Zero(t, got.buf.Len())
		require.LessOrEqual(t, got.buf.Cap(), maxPooledBufferCapacity)
		if capacity > maxPooledBufferCapacity {
			require.NotSame(t, fw, got, "oversized writers must not be pooled")
		}
		require.NoError(t, got.Write([]byte("next job")))
		require.Equal(t, "next job", got.buf.String())
		PutBufferWriterToPool(got)
	}
}
