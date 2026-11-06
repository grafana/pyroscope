package profiledump

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	thanosobjstore "github.com/thanos-io/objstore"

	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/s3"
)

type tenantEncryption struct{ tenants []string }

func (p *tenantEncryption) S3SSEType(tenant string) string {
	p.tenants = append(p.tenants, tenant)
	return s3.SSEKMS
}
func (p *tenantEncryption) S3SSEKMSKeyID(tenant string) string {
	p.tenants = append(p.tenants, tenant)
	return "key-" + tenant
}
func (p *tenantEncryption) S3SSEKMSEncryptionContext(tenant string) string {
	p.tenants = append(p.tenants, tenant)
	return `{"tenant":"` + tenant + `"}`
}

type recordingUploadBucket struct {
	thanosobjstore.Bucket
	keys []string
}

func (b *recordingUploadBucket) Upload(ctx context.Context, key string, body io.Reader, opts ...thanosobjstore.ObjectUploadOption) error {
	b.keys = append(b.keys, key)
	return b.Bucket.Upload(ctx, key, body, opts...)
}

func TestBucketUploadTenantAndPrefix(t *testing.T) {
	raw := &recordingUploadBucket{Bucket: thanosobjstore.NewInMemBucket()}
	bucket := objstore.NewPrefixedBucket(objstore.NewBucket(raw), "customer/prefix")
	provider := &tenantEncryption{}
	upload := BucketUpload(bucket, provider)
	for _, tenant := range []string{"a", "b"} {
		payloadKey, _, err := NewObjectKey(tenant, recorderNow)
		require.NoError(t, err)
		for _, key := range []string{payloadKey, strings.TrimSuffix(payloadKey, ".pprof") + ".json"} {
			provider.tenants = nil
			require.NoError(t, upload(context.Background(), tenant, key, strings.NewReader("payload")))
			require.Equal(t, []string{tenant, tenant, tenant}, provider.tenants)
			require.Equal(t, "customer/prefix/"+key, raw.keys[len(raw.keys)-1])
			body, err := raw.Get(context.Background(), "customer/prefix/"+key)
			require.NoError(t, err)
			data, err := io.ReadAll(body)
			require.NoError(t, err)
			require.NoError(t, body.Close())
			require.Equal(t, "payload", string(data))
		}
	}
}
