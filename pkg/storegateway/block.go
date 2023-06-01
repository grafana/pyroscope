package storegateway

import (
	"context"
	"os"
	"path/filepath"

	"github.com/go-kit/log"
	"github.com/pkg/errors"

	"github.com/grafana/phlare/pkg/phlaredb"
	"github.com/grafana/phlare/pkg/phlaredb/block"
)

type BlockCloser interface {
	phlaredb.Querier
	Close() error
}

type Block struct {
	BlockCloser
	meta   *block.Meta
	logger log.Logger
}

func (bs *BucketStore) createBlock(ctx context.Context, meta *block.Meta) (*Block, error) {
	blockLocalPath := bs.localPath(meta.ULID.String())
	// add the dir if it doesn't exist
	if _, err := os.Stat(blockLocalPath); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(blockLocalPath, 0o750); err != nil {
			return nil, errors.Wrap(err, "create dir")
		}
	}
	metaPath := filepath.Join(blockLocalPath, block.MetaFilename)
	if _, err := os.Stat(metaPath); errors.Is(err, os.ErrNotExist) {
		// add meta.json if it does not exist
		if _, err := meta.WriteToFile(bs.logger, blockLocalPath); err != nil {
			return nil, errors.Wrap(err, "write meta.json")
		}
	} else {
		// read meta.json if it exists and validate it
		if diskMeta, _, err := block.MetaFromDir(blockLocalPath); err != nil {
			if meta.String() != diskMeta.String() {
				return nil, errors.Wrap(err, "meta.json does not match")
			}
			return nil, errors.Wrap(err, "read meta.json")
		}
	}

	return &Block{
		meta:        meta,
		logger:      bs.logger,
		BlockCloser: phlaredb.NewSingleBlockQuerierFromMeta(ctx, bs.bucket, meta),
	}, nil
}
