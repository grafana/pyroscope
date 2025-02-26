package util

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsyncWriter_Write(t *testing.T) {
	var buf bytes.Buffer
	writer := NewAsyncWriter(&buf, 10, 2, 2, 100*time.Millisecond)
	n, err := writer.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	writer.Close()
	assert.Equal(t, "hello", buf.String())
}

func TestAsyncWriter_Empty(t *testing.T) {
	var buf bytes.Buffer
	writer := NewAsyncWriter(&buf, 10, 2, 2, 100*time.Millisecond)
	writer.Close()
	assert.EqualValues(t, 0, buf.Len())
}

func TestAsyncWriter_Overflow(t *testing.T) {
	var buf bytes.Buffer
	writer := NewAsyncWriter(&buf, 10, 2, 2, 100*time.Millisecond)
	_, _ = writer.Write([]byte("hello"))
	_, _ = writer.Write([]byte("world"))
	writer.Close()
	assert.Equal(t, "helloworld", buf.String())
}

func TestAsyncWriter_FlushInterval(t *testing.T) {
	var buf bytes.Buffer
	writer := NewAsyncWriter(&buf, 10, 2, 2, 10*time.Millisecond)
	defer writer.Close()
	_, _ = writer.Write([]byte("hello"))
	assert.Eventually(t,
		func() bool { return assert.Equal(t, "hello", buf.String()) },
		time.Second, 10*time.Millisecond,
	)
}

func TestAsyncWriter_Close(t *testing.T) {
	var buf bytes.Buffer
	writer := NewAsyncWriter(&buf, 10, 2, 2, 100*time.Millisecond)
	_, _ = writer.Write([]byte("hello"))
	assert.NoError(t, writer.Close())
	assert.Equal(t, "hello", buf.String())
	assert.NoError(t, writer.Close())
}

func TestAsyncWriter_ConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	writer := NewAsyncWriter(&buf, 10, 2, 2, 100*time.Millisecond)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = writer.Write([]byte("hello"))
		}(i)
	}
	wg.Wait()

	writer.Close()
	assert.Equal(t, 50, buf.Len())
}
