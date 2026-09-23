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
	"time"

	"github.com/stretchr/testify/require"
)

func legacyMetadata() *LegacyMetadata {
	return &LegacyMetadata{Name: `app"<&>日本語`, StartTime: time.Unix(10, 123).UTC(), EndTime: time.Unix(11, 456).UTC(), SampleRate: math.MaxUint32, Spy: "spy", Units: "samples", Aggregation: "sum", DeclaredFormat: "jfr"}
}

func TestLegacyMetadataRoundTripAndSizing(t *testing.T) {
	m := testMetadata(4)
	m.SourceProtocol, m.Legacy = SourceIngest, legacyMetadata()
	b, err := json.Marshal(m)
	require.NoError(t, err)
	require.Equal(t, int64(len(b)), encodedMetadataSize(m))
	encoded := testEnvelope(t, m, []byte("body"))
	var body bytes.Buffer
	restored, err := Decode(bytes.NewReader(encoded), &body, int64(len(encoded)))
	require.NoError(t, err)
	require.Equal(t, m, restored)
	require.Equal(t, "body", body.String())
	require.Error(t, Encode(io.Discard, m, strings.NewReader("body"), int64(len(encoded)-1)))
	_, err = Decode(bytes.NewReader(encoded), io.Discard, int64(len(encoded)-1))
	require.Error(t, err)
}

func TestLegacyMetadataBounds(t *testing.T) {
	for _, field := range []string{"name", "spy", "units", "aggregation", "declared_format"} {
		t.Run(field, func(t *testing.T) {
			m := testMetadata(0)
			m.SourceProtocol, m.Legacy = SourceIngest, legacyMetadata()
			fields := map[string]*string{"name": &m.Legacy.Name, "spy": &m.Legacy.Spy, "units": &m.Legacy.Units, "aggregation": &m.Legacy.Aggregation, "declared_format": &m.Legacy.DeclaredFormat}
			for _, bad := range []string{strings.Repeat("x", MaxTextBytes+1), "bad\n", "\xff"} {
				*fields[field] = bad
				require.Error(t, m.Validate())
			}
		})
	}
	m := testMetadata(0)
	m.SourceProtocol, m.Legacy = SourceIngest, legacyMetadata()
	m.Legacy.Name = strings.Repeat("x", 1024)
	m.Legacy.Spy = m.Legacy.Name
	m.Legacy.Units = m.Legacy.Name
	m.Legacy.Aggregation = m.Legacy.Name
	require.ErrorContains(t, m.Validate(), "legacy metadata exceeds")
	m.Legacy.DeclaredFormat = ""
	require.NoError(t, m.Validate())
	m.Legacy.StartTime = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	require.ErrorContains(t, m.Validate(), "profile time")
}

func TestEnvelopeVersionCompatibility(t *testing.T) {
	// Equivalent to a pre-extension capture: schema/header 1 and no legacy field.
	m := testMetadata(3)
	m.SchemaVersion = 1
	v1 := testEnvelope(t, m, []byte("old"))
	require.Equal(t, uint16(1), binary.BigEndian.Uint16(v1[8:]))
	restored, err := Decode(bytes.NewReader(v1), io.Discard, math.MaxInt64)
	require.NoError(t, err)
	require.Nil(t, restored.Legacy)
	m.SchemaVersion = 2
	v2 := testEnvelope(t, m, []byte("old"))
	restored, err = Decode(bytes.NewReader(v2), io.Discard, math.MaxInt64)
	require.NoError(t, err)
	require.Nil(t, restored.Legacy)
	// The previous reader rejects this header before JSON decoding (its Version=1).
	require.NotEqual(t, uint16(1), binary.BigEndian.Uint16(v2[8:]))
	for _, tc := range []struct {
		name           string
		header, schema uint16
		extra          string
	}{
		{"v1 legacy", 1, 1, `,"legacy":{}`}, {"v1 null legacy", 1, 1, `,"legacy":null`},
		{"v1 case folded legacy", 1, 1, `,"Legacy":null`},
		{"unknown v1", 1, 1, `,"future":true`}, {"unknown v2", 2, 2, `,"future":true`},
		{"unknown nested", 2, 2, `,"legacy":{"future":true}`},
		{"mismatch", 1, 2, ""}, {"unsupported header", 4, 2, ""}, {"unsupported schema", 2, 4, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m.SchemaVersion = tc.schema
			b, err := json.Marshal(m)
			require.NoError(t, err)
			b = append(b[:len(b)-1], []byte(tc.extra+"}")...)
			raw := rawEnvelope(b, []byte("old"))
			binary.BigEndian.PutUint16(raw[8:], tc.header)
			_, err = Decode(bytes.NewReader(raw), io.Discard, math.MaxInt64)
			require.Error(t, err)
		})
	}
	m.SourceProtocol, m.Legacy, m.SchemaVersion = SourceIngest, legacyMetadata(), 1
	require.Error(t, Encode(io.Discard, m, strings.NewReader("old"), math.MaxInt64))
}

func TestLegacyRecorderOwnershipAndAccounting(t *testing.T) {
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
	c.Metadata.SourceProtocol = SourceIngest
	c.Metadata.Legacy = legacyMetadata()
	c.Metadata.Labels = map[string]string{"__name__": "app"}
	expected := *c.Metadata.Legacy
	out := r.Capture(context.Background(), "a", c)
	require.True(t, out.Enqueued)
	require.Equal(t, out.Size+encodingReservation, retained(r))
	payload[0] = '!'
	c.Metadata.Legacy.Name = "changed"
	c.Metadata.Labels["__name__"] = "changed"
	gate <- struct{}{}
	b := await(t, stored)
	var restored bytes.Buffer
	m, err := Decode(bytes.NewReader(b), &restored, cfg.MaxObjectBytes)
	require.NoError(t, err)
	require.Equal(t, expected, *m.Legacy)
	require.Equal(t, "app", m.Labels["__name__"])
	require.Equal(t, "native", restored.String())
	require.NoError(t, r.Shutdown(context.Background()))
	assertReleased(t, r)
}

func TestLegacyMetadataAggregateWireBound(t *testing.T) {
	m := testMetadata(1)
	m.SourceProtocol, m.Legacy = SourceIngest, legacyMetadata()
	m.Labels = make(map[string]string)
	for i := 0; i < 8; i++ {
		m.Labels[strings.Repeat("<", i+1)] = strings.Repeat("&", 1000)
	}
	m.Legacy.Name = strings.Repeat("<", 1024)
	m.Legacy.Spy = m.Legacy.Name
	m.Legacy.Units = m.Legacy.Name
	m.Legacy.Aggregation = m.Legacy.Name
	m.Legacy.DeclaredFormat = ""
	require.NoError(t, m.Validate())
	b, err := json.Marshal(m)
	require.NoError(t, err)
	require.Equal(t, int64(len(b)), encodedMetadataSize(m))
	require.Greater(t, len(b), MaxMetadataSize)
	require.Error(t, Encode(io.Discard, m, strings.NewReader("x"), math.MaxInt64))
	_, err = Decode(bytes.NewReader(rawEnvelope(b, []byte("x"))), io.Discard, math.MaxInt64)
	require.Error(t, err)
	r, _, _ := recorderFixture(t, recorderTestConfig(), discardUpload, nil)
	out := r.Capture(context.Background(), "a", Candidate{Metadata: m, Payload: BytesPayload("x")})
	require.Equal(t, DropTooLarge, out.Reason)
	require.False(t, out.Enqueued)
	assertReleased(t, r)
}
