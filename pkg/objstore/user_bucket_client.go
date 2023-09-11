// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/bucket/user_bucket_client.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package objstore

// NewUserBucketClient returns a bucket client to use to access the storage on behalf of the provided user.
// The cfgProvider can be nil.
func NewUserBucketClient(userID string, bucket Bucket, cfgProvider TenantConfigProvider) InstrumentedBucket {
	// Inject the user/tenant prefix.
	bucket = NewPrefixedBucket(bucket, userID)

	// Inject the SSE config.
	return NewSSEBucketClient(userID, bucket, cfgProvider)
}
