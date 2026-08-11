package memory

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
)

func TestInMemBucket_ConditionalUploadRoundTrip(t *testing.T) {
	ctx := context.Background()
	bkt := NewInMemBucket()

	require.NoError(t, bkt.Upload(ctx, "obj", bytes.NewReader([]byte("v1"))))
	attrs, err := bkt.Attributes(ctx, "obj")
	require.NoError(t, err)
	require.NotNil(t, attrs.Version)

	// A conditional upload against the current version succeeds.
	require.NoError(t, bkt.Upload(ctx, "obj", bytes.NewReader([]byte("v2")), objstore.WithIfMatch(attrs.Version)))

	// The same, now-stale version is rejected.
	err = bkt.Upload(ctx, "obj", bytes.NewReader([]byte("v3")), objstore.WithIfMatch(attrs.Version))
	require.Error(t, err)
	require.True(t, bkt.IsConditionNotMetErr(err))
}

func TestInMemBucket_GetRangeZeroLength(t *testing.T) {
	ctx := context.Background()
	bkt := NewInMemBucket()
	bkt.Set("obj", []byte("payload"))

	// A zero-length read of an existing object yields an empty reader.
	reader, err := bkt.GetRange(ctx, "obj", 2, 0)
	require.NoError(t, err)
	defer reader.Close()
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Empty(t, data)

	// A missing object is still reported as not found.
	_, err = bkt.GetRange(ctx, "missing", 0, 0)
	require.Error(t, err)
	require.True(t, bkt.IsObjNotFoundErr(err))
}

func TestInMemBucket_Set(t *testing.T) {
	ctx := context.Background()
	bkt := NewInMemBucket()

	bkt.Set("obj", []byte("payload"))

	reader, err := bkt.Get(ctx, "obj")
	require.NoError(t, err)
	defer reader.Close()
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "payload", string(data))
	require.Equal(t, 1, len(bkt.Objects()))
}
