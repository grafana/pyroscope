// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/tsdb/tenant_deletion_mark_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package bucket

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"

	phlareobj "github.com/grafana/pyroscope/pkg/objstore"
)

func TestTenantDeletionMarkExists(t *testing.T) {
	const username = "user"

	for name, tc := range map[string]struct {
		objects map[string][]byte
		exists  bool
	}{
		"empty": {
			objects: nil,
			exists:  false,
		},

		"mark doesn't exist": {
			objects: map[string][]byte{
				"user/phlaredb/01EQK4QKFHVSZYVJ908Y7HH9E0/meta.json": []byte("data"),
			},
			exists: false,
		},

		"mark exists": {
			objects: map[string][]byte{
				"user/phlaredb/01EQK4QKFHVSZYVJ908Y7HH9E0/meta.json": []byte("data"),
				"user/phlaredb/" + TenantDeletionMarkPath:            []byte("data"),
			},
			exists: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			bkt := objstore.NewInMemBucket()
			// "upload" objects
			for objName, data := range tc.objects {
				require.NoError(t, bkt.Upload(context.Background(), objName, bytes.NewReader(data)))
			}

			res, err := TenantDeletionMarkExists(context.Background(), phlareobj.NewBucket(bkt), username)
			require.NoError(t, err)
			require.Equal(t, tc.exists, res)
		})
	}
}
