package profiledump

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func nativeMetadataFixture(t *testing.T) (string, NativeMetadata) {
	t.Helper()
	capturedAt := time.Date(2026, 9, 25, 12, 34, 56, 123456789, time.UTC)
	key, id, err := NewNativeObjectKey("3648", capturedAt)
	require.NoError(t, err)
	return key, NativeMetadata{
		SchemaVersion: NativeSchemaVersion, CapturedAt: capturedAt,
		TenantID: "3648", CaptureID: id.String(), SourceProtocol: SourceConnect,
		NativeFormat: FormatPprof, PayloadEncoding: "unknown", PayloadSize: 0,
		DistributorID: "distributor-1", PolicyFingerprint: strings.Repeat("a", 64),
		Labels: map[string]string{"service_name": "test"}, OriginalProfileID: "original-id",
	}
}

func TestNativeMetadataRoundTrip(t *testing.T) {
	t.Parallel()
	key, metadata := nativeMetadataFixture(t)
	parsed, err := ParseNativeObjectKey(key)
	require.NoError(t, err)
	for _, encoding := range []string{"identity", "gzip", "unknown"} {
		for _, size := range []int64{0, 42, math.MaxInt64} {
			t.Run(fmt.Sprintf("%s/%d", encoding, size), func(t *testing.T) {
				metadata.PayloadEncoding, metadata.PayloadSize = encoding, size
				b, err := MarshalNativeMetadata(key, metadata)
				require.NoError(t, err)
				require.NotContains(t, string(b), "activation_source")
				decoded, err := ReadNativeMetadata(bytes.NewReader(b), parsed.MetadataKey)
				require.NoError(t, err)
				require.Equal(t, metadata, decoded)
			})
		}
	}
	metadata.CapturedAt = metadata.CapturedAt.In(time.FixedZone("offset", -7*60*60))
	b, err := MarshalNativeMetadata(key, metadata)
	require.NoError(t, err)
	decoded, err := ReadNativeMetadata(bytes.NewReader(b), key)
	require.NoError(t, err)
	require.True(t, decoded.CapturedAt.Equal(metadata.CapturedAt))
	require.Equal(t, time.UTC, decoded.CapturedAt.Location())
}

func TestNativeMetadataValidation(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*NativeMetadata){
		"version":       func(m *NativeMetadata) { m.SchemaVersion++ },
		"tenant":        func(m *NativeMetadata) { m.TenantID = "other" },
		"unsafe tenant": func(m *NativeMetadata) { m.TenantID = "../3648" },
		"capture ID":    func(m *NativeMetadata) { m.CaptureID = m.CaptureID[:25] + "!" },
		"different ID same time": func(m *NativeMetadata) {
			last := "0"
			if strings.HasSuffix(m.CaptureID, last) {
				last = "1"
			}
			m.CaptureID = m.CaptureID[:25] + last
		},
		"millisecond mismatch": func(m *NativeMetadata) { m.CapturedAt = m.CapturedAt.Add(time.Millisecond) },
		"minute mismatch":      func(m *NativeMetadata) { m.CapturedAt = m.CapturedAt.Add(time.Minute) },
		"zero time":            func(m *NativeMetadata) { m.CapturedAt = time.Time{} },
		"source":               func(m *NativeMetadata) { m.SourceProtocol = "otlp" },
		"format":               func(m *NativeMetadata) { m.NativeFormat = "jfr" },
		"encoding":             func(m *NativeMetadata) { m.PayloadEncoding = "br" },
		"negative size":        func(m *NativeMetadata) { m.PayloadSize = -1 },
		"empty distributor":    func(m *NativeMetadata) { m.DistributorID = "" },
		"long distributor":     func(m *NativeMetadata) { m.DistributorID = strings.Repeat("d", MaxTextBytes+1) },
		"long original ID":     func(m *NativeMetadata) { m.OriginalProfileID = strings.Repeat("p", MaxTextBytes+1) },
		"invalid UTF-8":        func(m *NativeMetadata) { m.OriginalProfileID = "\xff" },
		"control":              func(m *NativeMetadata) { m.DistributorID = "d\n" },
		"fingerprint length":   func(m *NativeMetadata) { m.PolicyFingerprint = "a" },
		"fingerprint alphabet": func(m *NativeMetadata) { m.PolicyFingerprint = strings.Repeat("A", 64) },
		"label count": func(m *NativeMetadata) {
			m.Labels = make(map[string]string)
			for i := 0; i <= MaxLabels; i++ {
				m.Labels[fmt.Sprint(i)] = ""
			}
		},
		"label name":  func(m *NativeMetadata) { m.Labels = map[string]string{strings.Repeat("n", MaxLabelName+1): "v"} },
		"label value": func(m *NativeMetadata) { m.Labels = map[string]string{"n": strings.Repeat("v", MaxLabelValue+1)} },
		"label bytes": func(m *NativeMetadata) {
			m.Labels = make(map[string]string)
			for i := 0; i < 8; i++ {
				m.Labels[fmt.Sprint(i)] = strings.Repeat("v", MaxLabelValue)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			key, metadata := nativeMetadataFixture(t)
			mutate(&metadata)
			_, err := MarshalNativeMetadata(key, metadata)
			require.Error(t, err)
			b, err := json.Marshal(metadata)
			require.NoError(t, err)
			// encoding/json replaces invalid UTF-8. The encoder validation above
			// rejects it before that replacement. Raw invalid JSON bytes are tested below.
			if name != "invalid UTF-8" {
				_, err = ReadNativeMetadata(bytes.NewReader(b), key)
				require.Error(t, err)
			}
		})
	}
	key, metadata := nativeMetadataFixture(t)
	_, err := MarshalNativeMetadata("../"+key, metadata)
	require.Error(t, err)
}

func TestNativeMetadataJSONContract(t *testing.T) {
	t.Parallel()
	key, metadata := nativeMetadataFixture(t)
	b, err := MarshalNativeMetadata(key, metadata)
	require.NoError(t, err)
	valid := string(b)
	for name, input := range map[string]string{
		"empty": "", "null": "null", "array": "[]", "incomplete": "{", "truncated": valid[:len(valid)-1],
		"missing size":    strings.Replace(valid, `,"payload_size":0`, "", 1),
		"null size":       strings.Replace(valid, `"payload_size":0`, `"payload_size":null`, 1),
		"string size":     strings.Replace(valid, `"payload_size":0`, `"payload_size":"0"`, 1),
		"fractional size": strings.Replace(valid, `"payload_size":0`, `"payload_size":0.5`, 1),
		"overflow size":   strings.Replace(valid, `"payload_size":0`, `"payload_size":9223372036854775808`, 1),
		"trailing object": valid + "{}", "trailing null": valid + "null", "trailing junk": valid + "x",
		"path redirection": strings.TrimSuffix(valid, "}") + `,"payload_key":"other/secret.pprof"}`,
		"old field":        strings.TrimSuffix(valid, "}") + `,"activation_source":"runtime_override"}`,
		"invalid UTF-8":    strings.Replace(valid, "distributor-1", "\xff", 1),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ReadNativeMetadata(strings.NewReader(input), key)
			require.Error(t, err)
		})
	}
	decoded, err := ReadNativeMetadata(strings.NewReader(" \n"+valid+"\r\n\t"), key)
	require.NoError(t, err)
	require.Equal(t, metadata, decoded)
	_, err = ReadNativeMetadata(bytes.NewReader(b), strings.Replace(key, "/3648/", "/other/", 1))
	require.ErrorContains(t, err, "identity")
	_, err = ReadNativeMetadata(bytes.NewReader(b), "../"+key)
	require.Error(t, err)
}

func TestNativeMetadataSerializedBounds(t *testing.T) {
	t.Parallel()
	key, metadata := nativeMetadataFixture(t)
	metadata.DistributorID = strings.Repeat("<", MaxTextBytes)
	metadata.OriginalProfileID = strings.Repeat("&", MaxTextBytes)
	metadata.Labels = make(map[string]string)
	for i := 0; i < 8; i++ {
		metadata.Labels[fmt.Sprint(i)] = strings.Repeat(">", MaxLabelValue-1)
	}
	b, err := MarshalNativeMetadata(key, metadata)
	require.NoError(t, err)
	// Bounded raw fields still expand substantially under ordinary JSON escaping.
	require.Greater(t, len(b), 6*(MaxLabelBytes-8+2*MaxTextBytes))
	require.LessOrEqual(t, len(b), MaxMetadataSize)
	decoded, err := ReadNativeMetadata(bytes.NewReader(b), key)
	require.NoError(t, err)
	require.Equal(t, metadata, decoded)
	// Whitespace counts toward the serialized bound, including trailing whitespace.
	atLimit := append(bytes.Clone(b), bytes.Repeat([]byte(" "), MaxMetadataSize-len(b))...)
	_, err = ReadNativeMetadata(bytes.NewReader(atLimit), key)
	require.NoError(t, err)
	reader := bytes.NewReader(append(atLimit, bytes.Repeat([]byte(" "), 100)...))
	_, err = ReadNativeMetadata(reader, key)
	require.ErrorContains(t, err, "length")
	require.Equal(t, 99, reader.Len(), "must consume at most the bound plus one byte")
}

type nativeMetadataErrorReader struct{ err error }

func (r nativeMetadataErrorReader) Read([]byte) (int, error) { return 0, r.err }

func TestNativeMetadataReadFailure(t *testing.T) {
	t.Parallel()
	key, metadata := nativeMetadataFixture(t)
	b, err := MarshalNativeMetadata(key, metadata)
	require.NoError(t, err)
	readErr := errors.New("storage read failed")
	_, err = ReadNativeMetadata(io.MultiReader(bytes.NewReader(b), nativeMetadataErrorReader{readErr}), key)
	require.ErrorIs(t, err, readErr)
}
