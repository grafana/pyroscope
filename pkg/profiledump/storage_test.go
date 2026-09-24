package profiledump

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/flagext"
	"github.com/stretchr/testify/require"
	thanosobjstore "github.com/thanos-io/objstore"

	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/s3"
)

type observedBucket struct {
	objstore.Bucket
	closes atomic.Int64
}

func (b *observedBucket) Close() error {
	b.closes.Add(1)
	return b.Bucket.Close()
}

type tenantEncryption struct{}

func (tenantEncryption) S3SSEType(string) string            { return s3.SSEKMS }
func (tenantEncryption) S3SSEKMSKeyID(tenant string) string { return "key-" + tenant }
func (tenantEncryption) S3SSEKMSEncryptionContext(tenant string) string {
	return `{"tenant":"` + tenant + `"}`
}

func TestRecorderBucketUpload(t *testing.T) {
	bucket := &observedBucket{Bucket: objstore.NewBucket(thanosobjstore.NewInMemBucket())}
	t.Cleanup(func() { require.NoError(t, bucket.Close()) })
	r, _, _ := recorderFixture(t, recorderTestConfig(), BucketUpload(bucket, nil), nil)
	payload := []byte{0x1f, 0x8b, 0, 0xff, 9}
	out := r.Capture(context.Background(), "a", candidate(payload))
	require.True(t, out.Enqueued)
	require.NoError(t, r.Shutdown(context.Background()))
	require.Zero(t, bucket.closes.Load(), "recorder must not close shared storage")
	stored, err := bucket.Get(context.Background(), out.ObjectKey)
	require.NoError(t, err)
	defer func() { require.NoError(t, stored.Close()) }()
	var restored bytes.Buffer
	metadata, err := Decode(stored, &restored, r.cfg.MaxObjectBytes)
	require.NoError(t, err)
	require.Equal(t, payload, restored.Bytes())
	require.Equal(t, "a", metadata.TenantID)
	assertReleased(t, r)
}

func TestRecorderTenantEncryptionReachesS3(t *testing.T) {
	type request struct {
		header http.Header
		path   string
	}
	requests := make(chan request, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.Copy(io.Discard, r.Body)
		if err != nil {
			t.Error(err)
		}
		requests <- request{r.Header.Clone(), r.URL.Path}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	client, err := s3.NewBucketClient(s3.Config{
		Endpoint: server.Listener.Addr().String(), Region: "test", BucketName: "customer",
		SecretAccessKey: flagext.SecretWithValue("test"), AccessKeyID: "test", Insecure: true,
	}, "test", log.NewNopLogger())
	require.NoError(t, err)
	bucket := &observedBucket{Bucket: objstore.NewBucket(client)}
	t.Cleanup(func() { require.NoError(t, bucket.Close()) })
	r, _, _ := recorderFixture(t, recorderTestConfig(), BucketUpload(bucket, tenantEncryption{}), nil)
	for _, tenant := range []string{"a", "b"} {
		out := r.Capture(context.Background(), tenant, candidate([]byte("native")))
		require.True(t, out.Enqueued)
		got := await(t, requests)
		require.Equal(t, "/customer/"+out.ObjectKey, got.path)
		require.Equal(t, "aws:kms", got.header.Get("x-amz-server-side-encryption"))
		require.Equal(t, "key-"+tenant, got.header.Get("x-amz-server-side-encryption-aws-kms-key-id"))
		wantContext := base64.StdEncoding.EncodeToString([]byte(tenantEncryption{}.S3SSEKMSEncryptionContext(tenant)))
		require.Equal(t, wantContext, got.header.Get("x-amz-server-side-encryption-context"))
	}
	require.NoError(t, r.Shutdown(context.Background()))
	require.Zero(t, bucket.closes.Load())
	assertReleased(t, r)
}
