package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

type trackedReplayInput struct {
	*bytes.Reader
	closed bool
}

func (r *trackedReplayInput) Close() error {
	r.closed = true
	return nil
}

func TestReplayInputCloseDoesNotDrain(t *testing.T) {
	raw := &trackedReplayInput{Reader: bytes.NewReader(bytes.Repeat([]byte("profile"), 10000))}
	r := newRawHashReadCloser(raw)
	_, err := r.Read(make([]byte, 1))
	require.NoError(t, err)
	remaining := raw.Len()
	require.Positive(t, remaining)
	require.NoError(t, r.Close())
	require.True(t, raw.closed)
	require.Equal(t, remaining, raw.Len(), "closing must not read the remaining input")
}

func TestReplayInputCompleteChecksum(t *testing.T) {
	data := bytes.Repeat([]byte("profile"), 10000)
	for _, compressed := range []bool{false, true} {
		t.Run(fmt.Sprintf("compressed=%t", compressed), func(t *testing.T) {
			encoded := data
			if compressed {
				encoder, err := zstd.NewWriter(nil)
				require.NoError(t, err)
				encoded = encoder.EncodeAll(data, nil)
				encoder.Close()
			}
			r := newRawHashReadCloser(io.NopCloser(bytes.NewReader(encoded)))
			decoded, err := io.ReadAll(r)
			require.NoError(t, err)
			require.Equal(t, data, decoded)
			require.NoError(t, r.Close())
			require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(encoded)), r.HexSum())
		})
	}
}
