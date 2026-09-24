package profiledump

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPreparedEnvelope(t *testing.T) {
	t.Parallel()
	m := testMetadata(3)
	m.Labels = map[string]string{"escapes": "<>&\"\\\u2028\u2029"}
	metadataJSON, err := json.Marshal(m)
	require.NoError(t, err)
	size := int64(HeaderSize + len(metadataJSON) + 3)
	prepared, err := prepareEnvelope(m, size)
	require.NoError(t, err)
	require.Equal(t, size, prepared.objectSize)
	require.Equal(t, metadataJSON, prepared.metadataJSON)
	_, err = prepareEnvelope(m, size-1)
	require.Error(t, err)

	// Preparation owns serialized bytes and does not retain the caller's map.
	m.Labels["escapes"] = "changed"
	m.PayloadSize = 100
	var output bytes.Buffer
	require.NoError(t, prepared.encode(&output, bytes.NewReader([]byte{1, 2, 3})))
	require.Equal(t, size, int64(output.Len()))
	require.Equal(t, metadataJSON, output.Bytes()[HeaderSize:HeaderSize+len(metadataJSON)])
	_, err = Decode(bytes.NewReader(output.Bytes()), io.Discard, size)
	require.NoError(t, err)
}

func TestEnvelopeSizeArithmetic(t *testing.T) {
	t.Parallel()
	const metadataSize = int64(MaxMetadataSize)
	const largestPayload = math.MaxInt64 - int64(HeaderSize) - metadataSize
	size, err := envelopeSize(metadataSize, largestPayload, math.MaxInt64)
	require.NoError(t, err)
	require.Equal(t, int64(math.MaxInt64), size)
	for _, tc := range []struct{ metadata, payload, limit int64 }{
		{metadataSize, largestPayload + 1, math.MaxInt64},
		{metadataSize, math.MaxInt64, math.MaxInt64},
		{math.MaxInt64, 0, math.MaxInt64},
		{MaxMetadataSize + 1, 0, math.MaxInt64},
		{0, 0, math.MaxInt64},
		{-1, 0, math.MaxInt64},
		{1, -1, math.MaxInt64},
		{1, 0, 0},
		{1, 0, -1},
		{metadataSize, largestPayload, math.MaxInt64 - 1},
	} {
		_, err := envelopeSize(tc.metadata, tc.payload, tc.limit)
		require.Error(t, err, "%+v", tc)
	}
}

func BenchmarkPrepareEnvelope(b *testing.B) {
	m := testMetadata(1 << 20)
	m.Labels["service_name"] = strings.Repeat("<", 1000)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := prepareEnvelope(m, 2<<20); err != nil {
			b.Fatal(err)
		}
	}
}
