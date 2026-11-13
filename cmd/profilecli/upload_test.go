package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadFileReadError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing.pprof")
	transport := &uploadCountingTransport{}
	err := upload(context.Background(), &uploadParams{
		phlareClient: &phlareClient{
			URL:              "http://localhost:4040",
			defaultTransport: transport,
		},
		paths: []string{path},
	})

	assert.Zero(t, transport.requests)
	require.ErrorIs(t, err, os.ErrNotExist)
	var pathErr *os.PathError
	require.ErrorAs(t, err, &pathErr)
	assert.Equal(t, path, pathErr.Path)
}

type uploadCountingTransport struct {
	requests int
}

func (t *uploadCountingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.requests++
	return nil, os.ErrInvalid
}
