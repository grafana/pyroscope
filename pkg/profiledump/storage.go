package profiledump

import (
	"context"
	"io"

	"github.com/grafana/pyroscope/v2/pkg/objstore"
)

// BucketUpload borrows the existing customer bucket and applies current tenant SSE
// settings without adding a tenant prefix to the complete capture key.
// It leaves the shared bucket open.
func BucketUpload(bucket objstore.Bucket, tenants objstore.TenantConfigProvider) UploadFunc {
	return func(ctx context.Context, tenant, key string, body io.Reader) error {
		return objstore.NewSSEBucketClient(tenant, bucket, tenants).Upload(ctx, key, body)
	}
}
