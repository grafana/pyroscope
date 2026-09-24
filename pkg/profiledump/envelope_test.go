package profiledump

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

func testMetadata(size int64) Metadata {
	timestamp := time.Date(2026, 9, 16, 12, 34, 56, 123456789, time.UTC)
	return Metadata{
		SchemaVersion: Version, CapturedAt: timestamp, TenantID: "tenant/a",
		SourceProtocol: SourceConnect, NativeFormat: FormatPprof, PayloadEncoding: "gzip",
		Labels:            map[string]string{"service_name": "checkout", "region": "west", "empty": ""},
		OriginalProfileID: "sample-123", DistributorID: "distributor-1",
		ActivationSource: ActivationRuntimeOverride, PolicyFingerprint: strings.Repeat("a1", 32),
		CaptureID:   ulid.MustNew(ulid.Timestamp(timestamp), bytes.NewReader(make([]byte, 10))).String(),
		PayloadSize: size,
	}
}

func testEnvelope(t *testing.T, m Metadata, payload []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, Encode(&b, m, bytes.NewReader(payload), math.MaxInt64))
	return b.Bytes()
}

// Build deliberately invalid envelopes without invoking encoder validation.
func rawEnvelope(metadata, payload []byte) []byte {
	b := make([]byte, HeaderSize, HeaderSize+len(metadata)+len(payload))
	copy(b, Magic)
	binary.BigEndian.PutUint16(b[len(Magic):], Version)
	binary.BigEndian.PutUint32(b[len(Magic)+2:], uint32(len(metadata)))
	b = append(b, metadata...)
	return append(b, payload...)
}

func TestEnvelopeRoundTrip(t *testing.T) {
	t.Parallel()
	var compressed bytes.Buffer
	z := gzip.NewWriter(&compressed)
	_, err := z.Write([]byte{0x0a, 0x02, 0x08, 0x01})
	require.NoError(t, err)
	require.NoError(t, z.Close())
	for _, payload := range [][]byte{compressed.Bytes(), {0x1f, 0x8b, 0x08, 0, 0xff, 0, 42}, {0xff, 0x00}, {}} {
		m := testMetadata(int64(len(payload)))
		b := testEnvelope(t, m, payload)
		var restored bytes.Buffer
		got, err := Decode(bytes.NewReader(b), &restored, int64(len(b)))
		require.NoError(t, err)
		require.Equal(t, m, got)
		require.Equal(t, payload, restored.Bytes()[:len(payload)])
		require.Equal(t, payload, b[len(b)-len(payload):])
	}
}

func TestEnvelopeTruncation(t *testing.T) {
	t.Parallel()
	b := testEnvelope(t, testMetadata(4), []byte{0, 1, 2, 3})
	for n := 0; n < len(b); n++ {
		_, err := Decode(bytes.NewReader(b[:n]), io.Discard, math.MaxInt64)
		require.Error(t, err, "accepted prefix of length %d", n)
	}
}

func TestEnvelopeHeaderValidation(t *testing.T) {
	t.Parallel()
	valid := testEnvelope(t, testMetadata(1), []byte{42})
	for _, tc := range []struct {
		name   string
		mutate func([]byte)
	}{
		{"magic", func(b []byte) { b[0] = 0 }},
		{"version zero", func(b []byte) { binary.BigEndian.PutUint16(b[8:], 0) }},
		{"version 2", func(b []byte) { binary.BigEndian.PutUint16(b[8:], 2) }},
		{"version 3", func(b []byte) { binary.BigEndian.PutUint16(b[8:], 3) }},
		{"unknown version", func(b []byte) { binary.BigEndian.PutUint16(b[8:], math.MaxUint16) }},
		{"zero metadata", func(b []byte) { binary.BigEndian.PutUint32(b[10:], 0) }},
		{"oversized metadata", func(b []byte) { binary.BigEndian.PutUint32(b[10:], MaxMetadataSize+1) }},
		{"uint32 maximum", func(b []byte) { binary.BigEndian.PutUint32(b[10:], math.MaxUint32) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := bytes.Clone(valid)
			tc.mutate(b)
			r := bytes.NewReader(b)
			_, err := Inspect(r, math.MaxInt64)
			require.Error(t, err)
			require.Equal(t, len(b)-HeaderSize, r.Len(), "must reject before reading metadata")
		})
	}
}

func TestEnvelopeSchemaVersionValidation(t *testing.T) {
	t.Parallel()
	for _, version := range []uint16{0, 2, 3, math.MaxUint16} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			m := testMetadata(0)
			m.SchemaVersion = version
			require.ErrorContains(t, m.Validate(), "unsupported metadata schema version")
			var output bytes.Buffer
			require.ErrorContains(t, Encode(&output, m, bytes.NewReader(nil), math.MaxInt64), "unsupported metadata schema version")
			require.Empty(t, output.Bytes())
			raw, err := json.Marshal(m)
			require.NoError(t, err)
			_, err = Inspect(bytes.NewReader(rawEnvelope(raw, nil)), math.MaxInt64)
			require.ErrorContains(t, err, "header and schema versions differ")
		})
	}
}

func TestEnvelopeInvalidJSON(t *testing.T) {
	t.Parallel()
	valid, err := json.Marshal(testMetadata(0))
	require.NoError(t, err)
	for name, raw := range map[string][]byte{
		"syntax":                []byte(`{"schema_version":`),
		"null":                  []byte(`null`),
		"array":                 []byte(`[]`),
		"empty object":          []byte(`{}`),
		"invalid UTF8":          append(bytes.Clone(valid), 0xff),
		"second JSON":           append(bytes.Clone(valid), []byte(` {}`)...),
		"junk":                  append(bytes.Clone(valid), 'x'),
		"invalid whitespace":    append(bytes.Clone(valid), '\v'),
		"unknown field":         bytes.Replace(valid, []byte(`"schema_version"`), []byte(`"headers":{},"schema_version"`), 1),
		"missing payload size":  bytes.Replace(valid, []byte(`,"payload_size":0`), nil, 1),
		"null payload size":     bytes.Replace(valid, []byte(`"payload_size":0`), []byte(`"payload_size":null`), 1),
		"negative payload size": bytes.Replace(valid, []byte(`"payload_size":0`), []byte(`"payload_size":-1`), 1),
		"int64 overflow":        bytes.Replace(valid, []byte(`"payload_size":0`), []byte(`"payload_size":9223372036854775808`), 1),
		"uint64 overflow":       bytes.Replace(valid, []byte(`"payload_size":0`), []byte(`"payload_size":18446744073709551616`), 1),
		"fractional size":       bytes.Replace(valid, []byte(`"payload_size":0`), []byte(`"payload_size":0.5`), 1),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Inspect(bytes.NewReader(rawEnvelope(raw, nil)), math.MaxInt64)
			require.Error(t, err)
		})
	}
	_, err = Inspect(bytes.NewReader(rawEnvelope(append(valid, ' ', '\n', '\t'), nil)), math.MaxInt64)
	require.NoError(t, err, "JSON whitespace is allowed within metadata length")
}

func TestEnvelopeInvalidMetadata(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*Metadata){
		"missing capture time":  func(m *Metadata) { m.CapturedAt = time.Time{} },
		"capture time mismatch": func(m *Metadata) { m.CapturedAt = m.CapturedAt.Add(time.Millisecond) },
		"capture ID":            func(m *Metadata) { m.CaptureID = "invalid" },
		"missing tenant":        func(m *Metadata) { m.TenantID = "" },
		"tenant size":           func(m *Metadata) { m.TenantID = strings.Repeat("a", MaxTenantBytes+1) },
		"source":                func(m *Metadata) { m.SourceProtocol = "invalid-source" },
		"format":                func(m *Metadata) { m.NativeFormat = "invalid-format" },
		"encoding":              func(m *Metadata) { m.PayloadEncoding = "invalid-encoding" },
		"profile ID size":       func(m *Metadata) { m.OriginalProfileID = strings.Repeat("a", MaxTextBytes+1) },
		"distributor missing":   func(m *Metadata) { m.DistributorID = "" },
		"distributor size":      func(m *Metadata) { m.DistributorID = strings.Repeat("a", MaxTextBytes+1) },
		"control characters":    func(m *Metadata) { m.DistributorID = "foo\nbar" },
		"activation":            func(m *Metadata) { m.ActivationSource = "invalid-activation" },
		"fingerprint length":    func(m *Metadata) { m.PolicyFingerprint = "abcd" },
		"fingerprint case":      func(m *Metadata) { m.PolicyFingerprint = strings.Repeat("A", 64) },
		"fingerprint hex":       func(m *Metadata) { m.PolicyFingerprint = strings.Repeat("z", 64) },
		"negative payload":      func(m *Metadata) { m.PayloadSize = -1 },
		"addition overflow":     func(m *Metadata) { m.PayloadSize = math.MaxInt64 },
		"label name empty":      func(m *Metadata) { m.Labels = map[string]string{"": "v"} },
		"label name size":       func(m *Metadata) { m.Labels = map[string]string{strings.Repeat("a", MaxLabelName+1): "v"} },
		"label value size":      func(m *Metadata) { m.Labels = map[string]string{"k": strings.Repeat("a", MaxLabelValue+1)} },
		"label count": func(m *Metadata) {
			for i := 0; i <= MaxLabels; i++ {
				m.Labels[fmt.Sprint(i)] = "v"
			}
		},
		"label total": func(m *Metadata) {
			for i := 0; i < 9; i++ {
				m.Labels[fmt.Sprint(i)] = strings.Repeat("a", 1024)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			m := testMetadata(0)
			mutate(&m)
			var out bytes.Buffer
			require.Error(t, Encode(&out, m, bytes.NewReader(nil), math.MaxInt64))
			require.Empty(t, out.Bytes(), "invalid metadata must fail before writing")
			raw, err := json.Marshal(m)
			require.NoError(t, err)
			_, err = Inspect(bytes.NewReader(rawEnvelope(raw, nil)), math.MaxInt64)
			require.Error(t, err)
		})
	}
	m := testMetadata(0)
	m.TenantID = "\xff"
	require.Error(t, m.Validate(), "encoder must not silently replace invalid UTF-8")
}

func TestEnvelopePayloadSizeAndLimits(t *testing.T) {
	t.Parallel()
	for _, size := range []int64{0, 1, 3, math.MaxInt64} {
		m := testMetadata(size)
		require.Error(t, Encode(io.Discard, m, bytes.NewReader([]byte{1, 2}), math.MaxInt64))
		raw, err := json.Marshal(m)
		require.NoError(t, err)
		_, err = Decode(bytes.NewReader(rawEnvelope(raw, []byte{1, 2})), io.Discard, math.MaxInt64)
		require.Error(t, err)
	}
	b := testEnvelope(t, testMetadata(2), []byte{1, 2})
	for _, limit := range []int64{-1, 0, int64(HeaderSize) - 1, int64(HeaderSize), int64(len(b)) - 1} {
		require.Error(t, Encode(io.Discard, testMetadata(2), bytes.NewReader([]byte{1, 2}), limit))
		_, err := Decode(bytes.NewReader(b), io.Discard, limit)
		require.Error(t, err)
	}
	_, err := Decode(bytes.NewReader(append(bytes.Clone(b), b...)), io.Discard, math.MaxInt64)
	require.Error(t, err, "concatenated envelopes are not permitted")
	_, err = Decode(bytes.NewReader(append(bytes.Clone(b), 0)), io.Discard, math.MaxInt64)
	require.Error(t, err, "even a single trailing byte is rejected")
}

func TestInspectDoesNotReadPayload(t *testing.T) {
	t.Parallel()
	b := testEnvelope(t, testMetadata(4), []byte{1, 2, 3, 4})
	r := bytes.NewReader(b)
	info, err := Inspect(r, int64(len(b)))
	require.NoError(t, err)
	require.Equal(t, 4, r.Len())
	require.Equal(t, int64(len(b)-4), info.PayloadOffset)
	require.Equal(t, int64(len(b)), info.ObjectSize)
	// A range ending at metadata is enough for inspection, but not decoding.
	prefix := b[:info.PayloadOffset]
	_, err = Inspect(io.MultiReader(bytes.NewReader(prefix), errorReader{}), int64(len(b)))
	require.NoError(t, err)
	_, err = Decode(bytes.NewReader(prefix), io.Discard, int64(len(b)))
	require.Error(t, err)
	// Large declarations do not cause payload-sized allocations or reads.
	m := testMetadata(1 << 50)
	raw, err := json.Marshal(m)
	require.NoError(t, err)
	_, err = Inspect(bytes.NewReader(rawEnvelope(raw, nil)), math.MaxInt64)
	require.NoError(t, err)
	_, err = Decode(bytes.NewReader(rawEnvelope(raw, nil)), io.Discard, math.MaxInt64)
	require.Error(t, err)
}

func TestMetadataWireBound(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(testMetadata(0))
	require.NoError(t, err)
	padded := append(raw, bytes.Repeat([]byte{' '}, MaxMetadataSize-len(raw))...)
	_, err = Inspect(bytes.NewReader(rawEnvelope(padded, nil)), math.MaxInt64)
	require.NoError(t, err)
	_, err = Inspect(bytes.NewReader(rawEnvelope(append(padded, ' '), nil)), math.MaxInt64)
	require.Error(t, err)

	// JSON expansion counts toward object admission, using actual encoded bytes.
	m := testMetadata(0)
	m.Labels = make(map[string]string)
	for i := 0; i < 8; i++ {
		m.Labels[fmt.Sprint(i)] = strings.Repeat("<", 1023)
	}
	m.TenantID = strings.Repeat("<", MaxTenantBytes)
	m.OriginalProfileID = strings.Repeat("<", MaxTextBytes)
	m.DistributorID = strings.Repeat("<", MaxTextBytes)
	require.NoError(t, m.Validate())
	raw, err = json.Marshal(m)
	require.NoError(t, err)
	require.Greater(t, len(raw), MaxLabelBytes*6)
	require.LessOrEqual(t, len(raw), MaxMetadataSize)
	var output bytes.Buffer
	require.Error(t, Encode(&output, m, bytes.NewReader(nil), int64(HeaderSize+len(raw)-1)))
	require.Empty(t, output.Bytes())
	require.NoError(t, Encode(&output, m, bytes.NewReader(nil), int64(HeaderSize+len(raw))))
}

var errTestIO = errors.New("test I/O failure")

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errTestIO }

type failWriter struct{ remaining int }

func (w *failWriter) Write(b []byte) (int, error) {
	n := min(w.remaining, len(b))
	w.remaining -= n
	if n != len(b) {
		return n, errTestIO
	}
	return n, nil
}

type shortWriter struct{}

func (shortWriter) Write(b []byte) (int, error) { return len(b) - 1, nil }

func TestEnvelopeIOErrors(t *testing.T) {
	t.Parallel()
	m := testMetadata(2)
	b := testEnvelope(t, m, []byte{1, 2})
	for _, remaining := range []int{0, HeaderSize, len(b) - 1} {
		err := Encode(&failWriter{remaining: remaining}, m, bytes.NewReader([]byte{1, 2}), math.MaxInt64)
		require.ErrorIs(t, err, errTestIO)
	}
	require.ErrorIs(t, Encode(shortWriter{}, m, bytes.NewReader([]byte{1, 2}), math.MaxInt64), io.ErrShortWrite)
	require.ErrorIs(t, Encode(io.Discard, m, errorReader{}, math.MaxInt64), errTestIO)
	require.ErrorIs(t, Encode(io.Discard, testMetadata(0), errorReader{}, math.MaxInt64), errTestIO)
	for _, prefix := range []int{0, HeaderSize, len(b) - 1, len(b)} {
		_, err := Decode(io.MultiReader(bytes.NewReader(b[:prefix]), errorReader{}), io.Discard, math.MaxInt64)
		require.ErrorIs(t, err, errTestIO)
	}
	_, err := Decode(bytes.NewReader(b), &failWriter{}, math.MaxInt64)
	require.ErrorIs(t, err, errTestIO)
}

func FuzzEnvelope(f *testing.F) {
	m := testMetadata(3)
	raw, err := json.Marshal(m)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(rawEnvelope(raw, []byte{0, 1, 255}))
	f.Add([]byte(Magic))
	f.Add(rawEnvelope([]byte(`{"payload_size":9223372036854775807}`), nil))
	f.Fuzz(func(t *testing.T, b []byte) {
		var payload bytes.Buffer
		m, err := Decode(bytes.NewReader(b), &payload, 1<<20)
		if err != nil {
			return
		}
		require.Equal(t, m.PayloadSize, int64(payload.Len()))
		if len(m.Labels) == 0 {
			m.Labels = nil // Empty optional labels are omitted when re-encoded.
		}
		encoded := testEnvelope(t, m, payload.Bytes())
		var restored bytes.Buffer
		got, err := Decode(bytes.NewReader(encoded), &restored, 1<<20)
		require.NoError(t, err)
		require.Equal(t, m, got)
		require.Equal(t, payload.Bytes(), restored.Bytes())
	})
}
