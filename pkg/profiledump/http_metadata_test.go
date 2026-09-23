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

func TestHTTPMetadataRoundTripAndSizing(t *testing.T) {
	for _, values := range [][]string{nil, {"gzip"}, {"GZIP", "br, gzip", "", "\xff\x00\r\n"}, {strings.Repeat("x", 2048)}} {
		for _, decompressed := range []bool{false, true} {
			m := testMetadata(4)
			m.SourceProtocol = SourceOTLPHTTP
			m.Stored.Encoding, m.Incoming.Encoding = encodingUnknown, encodingUnknown
			m.HTTP = &HTTPMetadata{ContentEncoding: values, GzipDecompressed: decompressed}
			b, err := json.Marshal(m)
			require.NoError(t, err)
			require.Equal(t, int64(len(b)), encodedMetadataSize(m))
			encoded := testEnvelope(t, m, []byte("body"))
			restored, err := Decode(bytes.NewReader(encoded), io.Discard, int64(len(encoded)))
			require.NoError(t, err)
			require.Equal(t, uint16(1), restored.SchemaVersion)
			require.Equal(t, m, restored)
			require.Error(t, Encode(io.Discard, m, strings.NewReader("body"), int64(len(encoded)-1)))
		}
	}
}

func TestHTTPMetadataWireBounds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		values []string
	}{
		{"oversized declaration", []string{strings.Repeat("x", MaxMetadataSize+1)}},
		{"aggregate overhead", make([]string, MaxMetadataSize)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := testMetadata(4)
			m.SourceProtocol = SourceOTLPHTTP
			m.HTTP = &HTTPMetadata{ContentEncoding: tc.values}
			require.NoError(t, m.Validate())
			require.Greater(t, httpMetadataSize(*m.HTTP), int64(MaxMetadataSize))

			b, err := json.Marshal(m.HTTP)
			require.ErrorContains(t, err, "HTTP metadata exceeds byte limit")
			require.Empty(t, b)

			var output bytes.Buffer
			require.ErrorContains(t, Encode(&output, m, strings.NewReader("body"), math.MaxInt64), "HTTP metadata exceeds byte limit")
			require.Empty(t, output.Bytes())
		})
	}
}

func TestHTTPMetadataJSONValidation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, extra string
		wantErr     bool
	}{
		{"absent", "", false},
		{"empty", `,"http":{}`, false},
		{"null", `,"http":null`, false},
		{"unknown nested field", `,"http":{"future":true}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := testMetadata(0)
			m.SourceProtocol = SourceOTLPHTTP
			raw, err := json.Marshal(m)
			require.NoError(t, err)
			raw = append(raw[:len(raw)-1], []byte(tc.extra+"}")...)
			_, err = Decode(bytes.NewReader(rawEnvelope(raw, nil)), io.Discard, math.MaxInt64)
			if tc.wantErr {
				require.ErrorContains(t, err, `unknown field "future"`)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
