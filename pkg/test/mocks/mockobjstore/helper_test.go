package mockobjstore

import (
	context "context"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClientMock_MockGet(t *testing.T) {
	expected := "body"

	m := NewMockBucketWithHelper(t)
	m.MockGet("test", expected, nil)

	// Run many goroutines all requesting the same mocked object and
	// ensure there's no race.
	wg := sync.WaitGroup{}
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			reader, err := m.Get(context.Background(), "test")
			require.NoError(t, err)

			actual, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, []byte(expected), actual)

			require.NoError(t, reader.Close())
		}()
	}

	wg.Wait()
}
