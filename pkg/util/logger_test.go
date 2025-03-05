package util

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsyncWriter_Write(t *testing.T) {
	var buf bytes.Buffer
	w := NewAsyncWriter(&buf, 10, 2, 2, 100*time.Millisecond)
	n, err := w.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.NoError(t, w.Close())
	assert.Equal(t, "hello", buf.String())
}

func TestAsyncWriter_Empty(t *testing.T) {
	var buf bytes.Buffer
	w := NewAsyncWriter(&buf, 10, 2, 2, 100*time.Millisecond)
	assert.NoError(t, w.Close())
	assert.EqualValues(t, 0, buf.Len())
}

func TestAsyncWriter_Overflow(t *testing.T) {
	var buf bytes.Buffer
	w := NewAsyncWriter(&buf, 10, 2, 2, 100*time.Millisecond)
	_, _ = w.Write([]byte("hello"))
	_, _ = w.Write([]byte("world"))
	assert.NoError(t, w.Close())
	assert.Equal(t, "helloworld", buf.String())
}

func TestAsyncWriter_Close(t *testing.T) {
	var buf bytes.Buffer
	w := NewAsyncWriter(&buf, 10, 2, 2, 100*time.Millisecond)
	_, _ = w.Write([]byte("hello"))
	assert.NoError(t, w.Close())
	assert.Equal(t, "hello", buf.String())
	assert.NoError(t, w.Close())
}

func TestAsyncWriter_ConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	w := NewAsyncWriter(&buf, 50, 2, 10, 100*time.Millisecond)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = fmt.Fprintf(w, "hello %d\n", i)
		}(i)
	}
	wg.Wait()

	assert.NoError(t, w.Close())
	assert.Equal(t, 80, buf.Len())
}
