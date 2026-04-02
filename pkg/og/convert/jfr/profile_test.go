package jfr

import (
	"bytes"
	"context"
	"mime/multipart"
	"os"
	"testing"

	"github.com/grafana/dskit/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/api/model/labelset"
	"github.com/grafana/pyroscope/pkg/og/convert/pprof/bench"
	"github.com/grafana/pyroscope/pkg/og/ingestion"
)

const testTenantID = "test-tenant"

type fixedMaxProfileSize int

func (l fixedMaxProfileSize) MaxProfileSizeBytes(_ string) int { return int(l) }

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

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	jfrField, err := w.CreateFormFile("jfr", "jfr")
	require.NoError(t, err)
	_, err = jfrField.Write(gzippedJFR)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	multipartBody := b.Bytes()

	p := &RawProfile{
		FormDataContentType: w.FormDataContentType(),
		RawData:             multipartBody,
	}
	result, err := p.ParseToPprof(testContext(), testMetadata(), fixedMaxProfileSize(32<<20))
	require.NoError(t, err)

	assert.Equal(t, len(multipartBody), result.ReceivedCompressedProfileSize)
	assert.Equal(t, len(rawJFR), result.ReceivedDecompressedProfileSize)
}
