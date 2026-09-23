package profiledump

import (
	"bytes"
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
	require.Equal(t, uint16(1), restored.SchemaVersion)
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

func TestLegacyMetadataJSONValidation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, extra string
		wantErr     bool
	}{
		{"absent", "", false},
		{"empty", `,"legacy":{}`, false},
		{"null", `,"legacy":null`, false},
		{"unknown nested field", `,"legacy":{"future":true}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := testMetadata(0)
			m.SourceProtocol = SourceIngest
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
}
