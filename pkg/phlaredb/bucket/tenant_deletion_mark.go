// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/tsdb/tenant_deletion_mark.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package bucket

import (
	"bytes"
	"context"
	"encoding/json"
	"path"
	"time"

	"github.com/go-kit/log/level"
	"github.com/pkg/errors"

	"github.com/grafana/pyroscope/pkg/objstore"
	util_log "github.com/grafana/pyroscope/pkg/util"
)

// Relative to user-specific prefix.
const TenantDeletionMarkPath = "markers/tenant-deletion-mark.json"

type TenantDeletionMark struct {
	// Unix timestamp when deletion marker was created.
	DeletionTime int64 `json:"deletion_time"`

	// Unix timestamp when cleanup was finished.
	FinishedTime int64 `json:"finished_time,omitempty"`
}

func NewTenantDeletionMark(deletionTime time.Time) *TenantDeletionMark {
	return &TenantDeletionMark{DeletionTime: deletionTime.Unix()}
}

// Checks for deletion mark for tenant. Errors other than "object not found" are returned.
func TenantDeletionMarkExists(ctx context.Context, bkt objstore.BucketReader, userID string) (bool, error) {
	markerFile := path.Join(userID, "phlaredb/", TenantDeletionMarkPath)

	return bkt.Exists(ctx, markerFile)
}

// Uploads deletion mark to the tenant location in the bucket.
func WriteTenantDeletionMark(ctx context.Context, bkt objstore.Bucket, userID string, cfgProvider objstore.TenantConfigProvider, mark *TenantDeletionMark) error {
	bkt = objstore.NewTenantBucketClient(userID, bkt, cfgProvider)

	data, err := json.Marshal(mark)
	if err != nil {
		return errors.Wrap(err, "serialize tenant deletion mark")
	}

	return errors.Wrap(bkt.Upload(ctx, TenantDeletionMarkPath, bytes.NewReader(data)), "upload tenant deletion mark")
}

// Returns tenant deletion mark for given user, if it exists. If it doesn't exist, returns nil mark, and no error.
func ReadTenantDeletionMark(ctx context.Context, bkt objstore.BucketReader, userID string) (*TenantDeletionMark, error) {
	markerFile := path.Join(userID, "phlaredb/", TenantDeletionMarkPath)

	r, err := bkt.Get(ctx, markerFile)
	if err != nil {
		if bkt.IsObjNotFoundErr(err) {
			return nil, nil
		}

		return nil, errors.Wrapf(err, "failed to read deletion mark object: %s", markerFile)
	}

	mark := &TenantDeletionMark{}
	err = json.NewDecoder(r).Decode(mark)

	// Close reader before dealing with decode error.
	if closeErr := r.Close(); closeErr != nil {
		level.Warn(util_log.Logger).Log("msg", "failed to close bucket reader", "err", closeErr)
	}

	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode deletion mark object: %s", markerFile)
	}

	return mark, nil
}
