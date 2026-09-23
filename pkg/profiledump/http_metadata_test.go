package profiledump

import (
	"bytes"
	"encoding/binary"
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
			m.Stored.Encoding = encodingUnknown
			m.HTTP = &HTTPMetadata{ContentEncoding: values, GzipDecompressed: decompressed}
			b, err := json.Marshal(m)
			require.NoError(t, err)
			require.Equal(t, int64(len(b)), encodedMetadataSize(m))
			encoded := testEnvelope(t, m, []byte("body"))
			restored, err := Decode(bytes.NewReader(encoded), io.Discard, int64(len(encoded)))
			require.NoError(t, err)
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

func TestHTTPMetadataVersionCompatibility(t *testing.T) {
	for _, version := range []uint16{1, 2, 3} {
		m := testMetadata(0)
		m.SchemaVersion, m.SourceProtocol = version, SourceOTLPHTTP
		b := testEnvelope(t, m, nil)
		_, err := Decode(bytes.NewReader(b), io.Discard, math.MaxInt64)
		require.NoError(t, err)
		for _, extra := range []string{`,"http":{}`, `,"HTTP":null`, `,"http":{"future":true}`} {
			raw, err := json.Marshal(m)
			require.NoError(t, err)
			raw = append(raw[:len(raw)-1], []byte(extra+"}")...)
			envelope := rawEnvelope(raw, nil)
			binary.BigEndian.PutUint16(envelope[8:], version)
			_, err = Decode(bytes.NewReader(envelope), io.Discard, math.MaxInt64)
			if version < 3 || strings.Contains(extra, "future") {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		}
		m.Stored.Encoding = encodingUnknown
		if version < 3 {
			require.Error(t, m.Validate())
		} else {
			require.NoError(t, m.Validate())
		}
		m.HTTP = &HTTPMetadata{}
		m.Stored.Encoding = encodingIdentity
		if version < 3 {
			require.Error(t, m.Validate())
		} else {
			require.NoError(t, m.Validate())
		}
	}
}
