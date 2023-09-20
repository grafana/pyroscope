// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/tsdb/bucketindex/markers.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package block

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/oklog/ulid"
	"github.com/pkg/errors"

	"github.com/grafana/pyroscope/pkg/objstore"
)

const (
	MarkersPathname = "markers"
)

func markFilepath(blockID ulid.ULID, markFilename string) string {
	return fmt.Sprintf("%s/%s-%s", MarkersPathname, blockID.String(), markFilename)
}

func isMarkFilename(name string, markFilename string) (ulid.ULID, bool) {
	parts := strings.SplitN(name, "-", 2)
	if len(parts) != 2 {
		return ulid.ULID{}, false
	}

	// Ensure the 2nd part matches the mark filename.
	if parts[1] != markFilename {
		return ulid.ULID{}, false
	}

	// Ensure the 1st part is a valid block ID.
	id, err := ulid.Parse(filepath.Base(parts[0]))
	return id, err == nil
}

// DeletionMarkFilepath returns the path, relative to the tenant's bucket location,
// of a block deletion mark in the bucket markers location.
func DeletionMarkFilepath(blockID ulid.ULID) string {
	return markFilepath(blockID, DeletionMarkFilename)
}

// IsDeletionMarkFilename returns whether the input filename matches the expected pattern
// of block deletion markers stored in the markers location.
func IsDeletionMarkFilename(name string) (ulid.ULID, bool) {
	return isMarkFilename(name, DeletionMarkFilename)
}

// NoCompactMarkFilepath returns the path, relative to the tenant's bucket location,
// of a no-compact block mark in the bucket markers location.
func NoCompactMarkFilepath(blockID ulid.ULID) string {
	return markFilepath(blockID, NoCompactMarkFilename)
}

// IsNoCompactMarkFilename returns true if input filename matches the expected
// pattern of block marker stored in the markers location.
func IsNoCompactMarkFilename(name string) (ulid.ULID, bool) {
	return isMarkFilename(name, NoCompactMarkFilename)
}

// ListBlockDeletionMarks looks for block deletion marks in the global markers location
// and returns a map containing all blocks having a deletion mark and their location in the
// bucket.
func ListBlockDeletionMarks(ctx context.Context, bkt objstore.BucketReader) (map[ulid.ULID]struct{}, error) {
	discovered := map[ulid.ULID]struct{}{}

	// Find all markers in the storage.
	err := bkt.Iter(ctx, MarkersPathname+"/", func(name string) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		if blockID, ok := IsDeletionMarkFilename(path.Base(name)); ok {
			discovered[blockID] = struct{}{}
		}

		return nil
	})

	return discovered, errors.Wrap(err, "list block deletion marks")
}
