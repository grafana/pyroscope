package phlaredb

import (
	"context"
	"path"

	"github.com/grafana/dskit/runutil"

	"github.com/grafana/pyroscope/pkg/objstore/client"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/util"
)

// ValidateLocalBlock validates the block in the given directory is readable.
func ValidateLocalBlock(ctx context.Context, dir string) error {
	meta, err := block.ReadMetaFromDir(dir)
	if err != nil {
		return err
	}

	bkt, err := client.NewBucket(ctx, client.Config{
		StorageBackendConfig: client.StorageBackendConfig{
			Backend: client.Filesystem,
			Filesystem: filesystem.Config{
				Directory: path.Dir(dir),
			},
		},
	}, "validate")
	if err != nil {
		return err
	}
	q := NewSingleBlockQuerierFromMeta(ctx, bkt, meta)
	defer runutil.CloseWithLogOnErr(util.Logger, q, "closing block querier")
	return q.Open(ctx)
}
