// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/tsdb/testutil/objstore.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package testutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/objstore/client"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
)

func NewFilesystemBucket(t testing.TB, ctx context.Context, storageDir string) (objstore.Bucket, string) {
	bkt, err := client.NewBucket(ctx, client.Config{
		StorageBackendConfig: client.StorageBackendConfig{
			Backend: client.Filesystem,
			Filesystem: filesystem.Config{
				Directory: storageDir,
			},
		},
	}, "test")
	require.NoError(t, err)

	return bkt, storageDir
}
