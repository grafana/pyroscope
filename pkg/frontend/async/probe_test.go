package async

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
)

func TestProbeConditionalWrites_InMemBucketPasses(t *testing.T) {
	require.NoError(t, ProbeConditionalWrites(context.Background(), objstore.NewInMemBucket()))
}

// ignoresConditionsBucket silently drops every write condition, like a
// backend that advertises If-Match without enforcing it.
type ignoresConditionsBucket struct {
	objstore.Bucket
	uploadCalls int
}

func (b *ignoresConditionsBucket) Upload(ctx context.Context, name string, r io.Reader, _ ...objstore.ObjectUploadOption) error {
	b.uploadCalls++
	return b.Bucket.Upload(ctx, name, r)
}

func TestProbeConditionalWrites_FailsWhenConditionsAreIgnored(t *testing.T) {
	bucket := &ignoresConditionsBucket{Bucket: objstore.NewInMemBucket()}
	err := ProbeConditionalWrites(context.Background(), bucket)
	require.ErrorContains(t, err, "unexpectedly succeeded")

	// A confirmed wrong verdict is a deterministic backend defect, not a
	// transient failure: it must not be retried. One probe attempt makes
	// exactly 3 Upload calls ("a", "b", "c").
	require.Equal(t, 3, bucket.uploadCalls)
}

// noVersionAttributesBucket supports conditional uploads but always reports
// a nil Version, like the vendored S3 client when an object's ETag is empty.
type noVersionAttributesBucket struct {
	objstore.Bucket
}

func (b *noVersionAttributesBucket) Attributes(ctx context.Context, name string) (objstore.ObjectAttributes, error) {
	attrs, err := b.Bucket.Attributes(ctx, name)
	if err != nil {
		return attrs, err
	}
	attrs.Version = nil
	return attrs, nil
}

func TestProbeConditionalWrites_FailsWhenVersionIsEmpty(t *testing.T) {
	err := ProbeConditionalWrites(context.Background(), &noVersionAttributesBucket{objstore.NewInMemBucket()})
	require.ErrorContains(t, err, "reported no version")
}

// transientlyFailingBucket fails its first failCount Uploads with a plain
// error unrelated to conditions, then delegates.
type transientlyFailingBucket struct {
	objstore.Bucket
	failCount int
	calls     int
}

func (b *transientlyFailingBucket) Upload(ctx context.Context, name string, r io.Reader, opts ...objstore.ObjectUploadOption) error {
	b.calls++
	if b.calls <= b.failCount {
		return errors.New("simulated transient upload error")
	}
	return b.Bucket.Upload(ctx, name, r, opts...)
}

func TestProbeConditionalWrites_RetriesTransientFailureThenSucceeds(t *testing.T) {
	bucket := &transientlyFailingBucket{Bucket: objstore.NewInMemBucket(), failCount: 2}
	require.NoError(t, ProbeConditionalWrites(context.Background(), bucket))
	// The first two probe attempts each fail on their very first Upload
	// call; the third attempt succeeds and runs all 3 of its own uploads.
	require.Equal(t, 5, bucket.calls)
}

// rejectsAllConditionsBucket fails every If-Match upload with
// condition-not-met, even against the current version
type rejectsAllConditionsBucket struct {
	objstore.Bucket
	uploadCalls int
}

var errProbeConditionAlwaysRejected = errors.New("stub: condition never met")

func (b *rejectsAllConditionsBucket) Upload(ctx context.Context, name string, r io.Reader, opts ...objstore.ObjectUploadOption) error {
	b.uploadCalls++
	if len(opts) > 0 {
		return errProbeConditionAlwaysRejected
	}
	return b.Bucket.Upload(ctx, name, r, opts...)
}

func (b *rejectsAllConditionsBucket) IsConditionNotMetErr(err error) bool {
	return errors.Is(err, errProbeConditionAlwaysRejected) || b.Bucket.IsConditionNotMetErr(err)
}

func TestProbeConditionalWrites_FailsWhenConditionsAreAlwaysRejected(t *testing.T) {
	bucket := &rejectsAllConditionsBucket{Bucket: objstore.NewInMemBucket()}
	err := ProbeConditionalWrites(context.Background(), bucket)
	require.ErrorContains(t, err, "unexpectedly rejected")

	// A deterministic verdict must not be retried: one probe attempt makes
	// exactly 2 Upload calls ("a", then the rejected conditional "b").
	require.Equal(t, 2, bucket.uploadCalls)
}
