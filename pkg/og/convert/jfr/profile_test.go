package jfr

import (
	"bytes"
	"context"
	"mime/multipart"
	"os"
	"testing"

	"github.com/grafana/dskit/user"
	jfrPprof "github.com/grafana/jfr-parser/pprof"
	"github.com/klauspost/compress/gzip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/api/model/labelset"
	"github.com/grafana/pyroscope/v2/pkg/og/convert/pprof/bench"
	"github.com/grafana/pyroscope/v2/pkg/og/ingestion"
)

const testTenantID = "test-tenant"

type fixedMaxProfileSize int

func (l fixedMaxProfileSize) MaxProfileSizeBytes(_ string) int         { return int(l) }
func (l fixedMaxProfileSize) MaxProfileSymbolValueLength(_ string) int { return 0 }
func (l fixedMaxProfileSize) MaxProfileStacktraceSamples(_ string) int { return 0 }

func testContext() context.Context {
	return user.InjectOrgID(context.Background(), testTenantID)
}

func testMetadata() ingestion.Metadata {
	return ingestion.Metadata{
		LabelSet: labelset.New(map[string]string{"__name__": "javaapp"}),
	}
}

// TestParseToPprof_PlainJFR_SizesSame verifies that when raw (uncompressed) JFR bytes are
// submitted directly, both ReceivedCompressedProfileSize and ReceivedDecompressedProfileSize
// equal len(RawData), since no decompression takes place.
func TestParseToPprof_PlainJFR_SizesSame(t *testing.T) {
	rawJFR, err := bench.ReadGzipFile("testdata/cortex-dev-01__kafka-0__cpu__0.jfr.gz")
	require.NoError(t, err)

	p := &RawProfile{RawData: rawJFR}
	result, err := p.ParseToPprof(testContext(), testMetadata(), fixedMaxProfileSize(32<<20))
	require.NoError(t, err)

	assert.Equal(t, len(rawJFR), result.ReceivedCompressedProfileSize)
	assert.Equal(t, len(rawJFR), result.ReceivedDecompressedProfileSize)
}

// TestParseToPprof_MultipartGzipJFR_SizesDiffer verifies that when a gzip-compressed JFR
// file is submitted as a multipart form field, ReceivedCompressedProfileSize reflects the
// raw multipart body length and ReceivedDecompressedProfileSize reflects the decompressed
// JFR bytes length (which should be larger due to compression).
func TestParseToPprof_MultipartGzipJFR_SizesDiffer(t *testing.T) {
	gzippedJFR, err := os.ReadFile("testdata/cortex-dev-01__kafka-0__cpu__0.jfr.gz")
	require.NoError(t, err)

	rawJFR, err := bench.ReadGzipFile("testdata/cortex-dev-01__kafka-0__cpu__0.jfr.gz")
	require.NoError(t, err)

	multipartBody, contentType := multipartJFR(t, gzippedJFR, nil)

	p := &RawProfile{
		FormDataContentType: contentType,
		RawData:             multipartBody,
	}
	result, err := p.ParseToPprof(testContext(), testMetadata(), fixedMaxProfileSize(32<<20))
	require.NoError(t, err)

	assert.Equal(t, len(multipartBody), result.ReceivedCompressedProfileSize)
	assert.Equal(t, len(rawJFR), result.ReceivedDecompressedProfileSize)
}

// TestParseToPprof_MultipartGzipJFR_ProfileSizeLimit covers the decompression size limit
// applied to the gzipped multipart fields. A limit of 0 means "no limit", matching the
// documented semantics of validation.max-profile-size-bytes and the pprof ingest path.
func TestParseToPprof_MultipartGzipJFR_ProfileSizeLimit(t *testing.T) {
	gzippedJFR, err := os.ReadFile("testdata/cortex-dev-01__kafka-0__cpu__0.jfr.gz")
	require.NoError(t, err)

	rawJFR, err := bench.ReadGzipFile("testdata/cortex-dev-01__kafka-0__cpu__0.jfr.gz")
	require.NoError(t, err)

	gzippedLabels := gzipBytes(t, marshalLabels(t))

	for _, tc := range []struct {
		name    string
		limit   fixedMaxProfileSize
		wantErr string
	}{
		{
			name:  "no limit",
			limit: 0,
		},
		{
			name:  "limit above decompressed size",
			limit: fixedMaxProfileSize(len(rawJFR)),
		},
		{
			name:    "limit below decompressed size",
			limit:   fixedMaxProfileSize(len(rawJFR) - 1),
			wantErr: "decompressed size exceeds maximum allowed size",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			multipartBody, contentType := multipartJFR(t, gzippedJFR, gzippedLabels)

			p := &RawProfile{
				FormDataContentType: contentType,
				RawData:             multipartBody,
			}
			result, err := p.ParseToPprof(testContext(), testMetadata(), tc.limit)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, len(rawJFR), result.ReceivedDecompressedProfileSize)
		})
	}
}

func marshalLabels(t *testing.T) []byte {
	t.Helper()

	snapshot := &jfrPprof.LabelsSnapshot{
		Strings:  map[int64]string{1: "region", 2: "eu-west-1"},
		Contexts: map[int64]*jfrPprof.Context{1: {Labels: map[int64]int64{1: 2}}},
	}
	b, err := snapshot.MarshalVT()
	require.NoError(t, err)
	return b
}

func gzipBytes(t *testing.T, b []byte) []byte {
	t.Helper()

	var out bytes.Buffer
	w := gzip.NewWriter(&out)
	_, err := w.Write(b)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return out.Bytes()
}

func multipartJFR(t *testing.T, jfr, labels []byte) ([]byte, string) {
	t.Helper()

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	jfrField, err := w.CreateFormFile("jfr", "jfr")
	require.NoError(t, err)
	_, err = jfrField.Write(jfr)
	require.NoError(t, err)
	if labels != nil {
		labelsField, err := w.CreateFormFile("labels", "labels")
		require.NoError(t, err)
		_, err = labelsField.Write(labels)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())

	return b.Bytes(), w.FormDataContentType()
}
