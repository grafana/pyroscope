// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/tsdb/bucketindex/markers_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package block

import (
	"context"
	"strings"
	"testing"

	"github.com/oklog/ulid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/objstore/testutil"
)

func TestDeletionMarkFilepath(t *testing.T) {
	id := ulid.MustNew(1, nil)

	assert.Equal(t, "markers/"+id.String()+"-deletion-mark.json", DeletionMarkFilepath(id))
}

func TestIsDeletionMarkFilename(t *testing.T) {
	expected := ulid.MustNew(1, nil)

	_, ok := IsDeletionMarkFilename("xxx")
	assert.False(t, ok)

	_, ok = IsDeletionMarkFilename("xxx-deletion-mark.json")
	assert.False(t, ok)

	_, ok = IsDeletionMarkFilename("tenant-deletion-mark.json")
	assert.False(t, ok)

	actual, ok := IsDeletionMarkFilename(expected.String() + "-deletion-mark.json")
	assert.True(t, ok)
	assert.Equal(t, expected, actual)
}

func TestNoCompactMarkFilepath(t *testing.T) {
	id := ulid.MustNew(1, nil)

	assert.Equal(t, "markers/"+id.String()+"-no-compact-mark.json", NoCompactMarkFilepath(id))
}

func TestIsNoCompactMarkFilename(t *testing.T) {
	expected := ulid.MustNew(1, nil)

	_, ok := IsNoCompactMarkFilename("xxx")
	assert.False(t, ok)

	_, ok = IsNoCompactMarkFilename("xxx-no-compact-mark.json")
	assert.False(t, ok)

	_, ok = IsNoCompactMarkFilename("tenant-no-compact-mark.json")
	assert.False(t, ok)

	actual, ok := IsNoCompactMarkFilename(expected.String() + "-no-compact-mark.json")
	assert.True(t, ok)
	assert.Equal(t, expected, actual)
}

func TestListBlockDeletionMarks(t *testing.T) {
	var (
		ctx    = context.Background()
		block1 = ulid.MustNew(1, nil)
		block2 = ulid.MustNew(2, nil)
		block3 = ulid.MustNew(3, nil)
	)

	t.Run("should return an empty map on empty bucket", func(t *testing.T) {
		bkt, _ := testutil.NewFilesystemBucket(t, ctx, t.TempDir())

		actualMarks, actualErr := ListBlockDeletionMarks(ctx, bkt)
		require.NoError(t, actualErr)
		assert.Empty(t, actualMarks)
	})

	t.Run("should return a map with the locations of the block deletion marks found", func(t *testing.T) {
		bkt, _ := testutil.NewFilesystemBucket(t, ctx, t.TempDir())

		require.NoError(t, bkt.Upload(ctx, DeletionMarkFilepath(block1), strings.NewReader("{}")))
		require.NoError(t, bkt.Upload(ctx, NoCompactMarkFilepath(block2), strings.NewReader("{}")))
		require.NoError(t, bkt.Upload(ctx, DeletionMarkFilepath(block3), strings.NewReader("{}")))

		actualMarks, actualErr := ListBlockDeletionMarks(ctx, bkt)
		require.NoError(t, actualErr)
		assert.Equal(t, map[ulid.ULID]struct{}{
			block1: {},
			block3: {},
		}, actualMarks)
	})
}
