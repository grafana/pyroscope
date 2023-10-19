package storegateway

import (
	"context"
	"os"
	"path"
	"path/filepath"

	"github.com/go-kit/log"
	"github.com/pkg/errors"

	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
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
	var outMeta *block.Meta
	if _, err := os.Stat(metaPath); errors.Is(err, os.ErrNotExist) {
		// fetch the meta from the bucket
		r, err := bs.bucket.Get(ctx, path.Join(meta.ULID.String(), block.MetaFilename))
		if err != nil {
			return nil, errors.Wrap(err, "get meta")
		}
		meta, err := block.Read(r)
		if err != nil {
			return nil, errors.Wrap(err, "read meta")
		}
		// add meta.json if it does not exist
		if _, err := meta.WriteToFile(bs.logger, blockLocalPath); err != nil {
			return nil, errors.Wrap(err, "write meta.json")
		}
		outMeta = meta.Clone()
	} else {
		// read meta.json if it exists and validate it
		diskMeta, _, err := block.MetaFromDir(blockLocalPath)
		if err != nil {
			return nil, errors.Wrap(err, "read meta.json")
		}

		if meta.ULID.String() != diskMeta.ULID.String() {
			return nil, errors.Wrap(err, "meta.json does not match")
		}
		outMeta = diskMeta.Clone()

	}

	if outMeta.Version == 0 || len(outMeta.Files) == 0 {
		return nil, errors.New("meta.json is empty")
	}

	return &Block{
		meta:        outMeta,
		logger:      bs.logger,
		BlockCloser: phlaredb.NewSingleBlockQuerierFromMeta(ctx, bs.bucket, outMeta),
	}, nil
}
