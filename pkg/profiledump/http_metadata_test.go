package profiledump

import (
	"bytes"
	"context"
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

func TestHTTPRecorderOwnershipAndAccounting(t *testing.T) {
	gate := make(chan struct{})
	stored := make(chan []byte, 1)
	cfg := recorderTestConfig()
	r, _, _ := recorderFixture(t, cfg, func(ctx context.Context, _, _ string, body io.Reader) error {
		select {
		case <-gate:
		case <-ctx.Done():
			return ctx.Err()
		}
		b, err := io.ReadAll(body)
		stored <- b
		return err
	}, nil)
	defer close(gate)
	payload := []byte("native")
	c := candidate(payload)
	c.Metadata.SourceProtocol = SourceOTLPHTTP
	c.Metadata.HTTP = &HTTPMetadata{ContentEncoding: []string{"gzip", "\xff"}, GzipDecompressed: true}
	out := r.Capture(context.Background(), "a", c)
	require.True(t, out.Enqueued)
	require.Equal(t, out.Size+encodingReservation+2*MaxMetadataSize, retained(r))
	c.Metadata.HTTP.ContentEncoding[0] = "modified encoding"
	c.Metadata.HTTP.GzipDecompressed = false
	payload[0] = '!'
	gate <- struct{}{}
	var body bytes.Buffer
	m, err := Decode(bytes.NewReader(await(t, stored)), &body, cfg.MaxObjectBytes)
	require.NoError(t, err)
	require.Equal(t, &HTTPMetadata{ContentEncoding: []string{"gzip", "\xff"}, GzipDecompressed: true}, m.HTTP)
	require.Equal(t, "native", body.String())
	require.NoError(t, r.Shutdown(context.Background()))
	assertReleased(t, r)
}

func TestHTTPMetadataBudget(t *testing.T) {
	for _, values := range [][]string{{strings.Repeat("x", MaxMetadataSize)}, make([]string, MaxMetadataSize)} {
		r, _, _ := recorderFixture(t, recorderTestConfig(), func(context.Context, string, string, io.Reader) error { t.Error("unexpected upload"); return nil }, nil)
		c := candidate([]byte("body"))
		c.Metadata.SourceProtocol = SourceOTLPHTTP
		c.Metadata.HTTP = &HTTPMetadata{ContentEncoding: values}
		out := r.Capture(context.Background(), "a", c)
		require.Equal(t, DropTooLarge, out.Reason)
		assertReleased(t, r)
	}
	cfg := recorderTestConfig()
	cfg.MaxRetainedBytes = encodingReservation + 2*MaxMetadataSize
	cfg.MaxObjectBytes = MaxMetadataSize
	r, _, _ := recorderFixture(t, cfg, func(context.Context, string, string, io.Reader) error { t.Error("unexpected upload"); return nil }, nil)
	c := candidate([]byte("body"))
	c.Metadata.SourceProtocol = SourceOTLPHTTP
	c.Metadata.HTTP = &HTTPMetadata{}
	require.Equal(t, DropByteBudget, r.Capture(context.Background(), "a", c).Reason)
	assertReleased(t, r)
}
