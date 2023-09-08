// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/grafana/mimir/blob/main/pkg/compactor/tenant_deletion_api_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.
package compactor

// import (
// 	"bytes"
// 	"context"
// 	"net/http"
// 	"net/http/httptest"
// 	"path"
// 	"testing"

// 	"github.com/grafana/dskit/services"
// 	"github.com/grafana/dskit/user"
// 	"github.com/stretchr/testify/require"
// 	"github.com/thanos-io/objstore"

// 	"github.com/grafana/mimir/pkg/storage/tsdb"
// )

// func TestDeleteTenant(t *testing.T) {
// 	bkt := objstore.NewInMemBucket()
// 	cfg := prepareConfig(t)
// 	c, _, _, _, _ := prepare(t, cfg, bkt)
// 	require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))
// 	t.Cleanup(stopServiceFn(t, c))

// 	{
// 		resp := httptest.NewRecorder()
// 		c.DeleteTenant(resp, &http.Request{})
// 		require.Equal(t, http.StatusUnauthorized, resp.Code)
// 	}

// 	{
// 		ctx := context.Background()
// 		ctx = user.InjectOrgID(ctx, "fake")

// 		req := &http.Request{}
// 		resp := httptest.NewRecorder()
// 		c.DeleteTenant(resp, req.WithContext(ctx))

// 		require.Equal(t, http.StatusOK, resp.Code)
// 		objs := bkt.Objects()
// 		require.NotNil(t, objs[path.Join("fake", tsdb.TenantDeletionMarkPath)])
// 	}
// }

// func TestDeleteTenantStatus(t *testing.T) {
// 	const username = "user"

// 	for name, tc := range map[string]struct {
// 		objects               map[string][]byte
// 		expectedBlocksDeleted bool
// 	}{
// 		"empty": {
// 			objects:               nil,
// 			expectedBlocksDeleted: true,
// 		},

// 		"no user objects": {
// 			objects: map[string][]byte{
// 				"different-user/01EQK4QKFHVSZYVJ908Y7HH9E0/meta.json": []byte("data"),
// 			},
// 			expectedBlocksDeleted: true,
// 		},

// 		"non-block files": {
// 			objects: map[string][]byte{
// 				"user/deletion-mark.json": []byte("data"),
// 			},
// 			expectedBlocksDeleted: true,
// 		},

// 		"block files": {
// 			objects: map[string][]byte{
// 				"user/01EQK4QKFHVSZYVJ908Y7HH9E0/meta.json": []byte("data"),
// 			},
// 			expectedBlocksDeleted: false,
// 		},
// 	} {
// 		t.Run(name, func(t *testing.T) {
// 			bkt := objstore.NewInMemBucket()
// 			// "upload" objects
// 			for objName, data := range tc.objects {
// 				require.NoError(t, bkt.Upload(context.Background(), objName, bytes.NewReader(data)))
// 			}

// 			cfg := prepareConfig(t)
// 			c, _, _, _, _ := prepare(t, cfg, bkt)
// 			require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))
// 			t.Cleanup(stopServiceFn(t, c))

// 			res, err := c.isBlocksForUserDeleted(context.Background(), username)
// 			require.NoError(t, err)
// 			require.Equal(t, tc.expectedBlocksDeleted, res)
// 		})
// 	}
// }
