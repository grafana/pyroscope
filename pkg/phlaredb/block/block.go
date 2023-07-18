package block

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/runutil"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/thanos-io/objstore"
)

const (
	IndexFilename        = "index.tsdb"
	ParquetSuffix        = ".parquet"
	DeletionMarkFilename = "deletion-mark.json"

	HostnameLabel = "__hostname__"
)

// DownloadMeta downloads only meta file from bucket by block ID.
// TODO(bwplotka): Differentiate between network error & partial upload.
func DownloadMeta(ctx context.Context, logger log.Logger, bkt objstore.Bucket, id ulid.ULID) (Meta, error) {
	rc, err := bkt.Get(ctx, path.Join(id.String(), MetaFilename))
	if err != nil {
		return Meta{}, errors.Wrapf(err, "meta.json bkt get for %s", id.String())
	}
	defer runutil.CloseWithLogOnErr(logger, rc, "download meta bucket client")

	var m Meta

	obj, err := io.ReadAll(rc)
	if err != nil {
		return Meta{}, errors.Wrapf(err, "read meta.json for block %s", id.String())
	}

	if err = json.Unmarshal(obj, &m); err != nil {
		return Meta{}, errors.Wrapf(err, "unmarshal meta.json for block %s", id.String())
	}

	return m, nil
}

func IsBlockDir(path string) (id ulid.ULID, ok bool) {
	id, err := ulid.Parse(filepath.Base(path))
	return id, err == nil
}

// upload uploads block from given block dir that ends with block id.
// It makes sure cleanup is done on error to avoid partial block uploads.
// TODO(bplotka): Ensure bucket operations have reasonable backoff retries.
// NOTE: Upload updates `meta.Thanos.File` section.
func upload(ctx context.Context, logger log.Logger, bkt objstore.Bucket, bdir string) error {
	df, err := os.Stat(bdir)
	if err != nil {
		return err
	}
	if !df.IsDir() {
		return errors.Errorf("%s is not a directory", bdir)
	}

	// Verify dir.
	id, err := ulid.Parse(df.Name())
	if err != nil {
		return errors.Wrap(err, "not a block dir")
	}

	meta, err := ReadFromDir(bdir)
	if err != nil {
		// No meta or broken meta file.
		return errors.Wrap(err, "read meta")
	}

	// ensure labels are initialized
	if meta.Labels == nil {
		meta.Labels = make(map[string]string)
	}

	// add hostname if available
	if hostname, err := os.Hostname(); err == nil {
		meta.Labels[HostnameLabel] = hostname
	}

	metaEncoded := strings.Builder{}
	if err != nil {
		return errors.Wrap(err, "gather meta file stats")
	}

	if _, err := meta.WriteTo(&metaEncoded); err != nil {
		return errors.Wrap(err, "encode meta file")
	}

	// loop through files
	for _, file := range meta.Files {
		if err := objstore.UploadFile(ctx, logger, bkt, path.Join(bdir, file.RelPath), path.Join(id.String(), file.RelPath)); err != nil {
			return cleanUp(logger, bkt, id, errors.Wrapf(err, "uploading file '%s'", file.RelPath))
		}
	}

	// Meta.json always need to be uploaded as a last item. This will allow to assume block directories without meta file to be pending uploads.
	if err := bkt.Upload(ctx, path.Join(id.String(), MetaFilename), strings.NewReader(metaEncoded.String())); err != nil {
		// Don't call cleanUp here. Despite getting error, meta.json may have been uploaded in certain cases,
		// and even though cleanUp will not see it yet, meta.json may appear in the bucket later.
		// (Eg. S3 is known to behave this way when it returns 503 "SlowDown" error).
		// If meta.json is not uploaded, this will produce partial blocks, but such blocks will be cleaned later.
		return errors.Wrap(err, "upload meta file")
	}

	return nil
}

// Upload uploads a TSDB block to the object storage. It verifies basic
// features of Thanos block.
func Upload(ctx context.Context, logger log.Logger, bkt objstore.Bucket, bdir string) error {
	return upload(ctx, logger, bkt, bdir)
}

func cleanUp(logger log.Logger, bkt objstore.Bucket, id ulid.ULID, err error) error {
	// Cleanup the dir with an uncancelable context.
	cleanErr := Delete(context.Background(), logger, bkt, id)
	if cleanErr != nil {
		return errors.Wrapf(err, "failed to clean block after upload issue. Partial block in system. Err: %s", err.Error())
	}
	return err
}

// Delete removes directory that is meant to be block directory.
// NOTE: Always prefer this method for deleting blocks.
//   - We have to delete block's files in the certain order (meta.json first and deletion-mark.json last)
//     to ensure we don't end up with malformed partial blocks. Thanos system handles well partial blocks
//     only if they don't have meta.json. If meta.json is present Thanos assumes valid block.
//   - This avoids deleting empty dir (whole bucket) by mistake.
func Delete(ctx context.Context, logger log.Logger, bkt objstore.Bucket, id ulid.ULID) error {
	metaFile := path.Join(id.String(), MetaFilename)
	deletionMarkFile := path.Join(id.String(), DeletionMarkFilename)

	// Delete block meta file.
	ok, err := bkt.Exists(ctx, metaFile)
	if err != nil {
		return errors.Wrapf(err, "stat %s", metaFile)
	}

	if ok {
		if err := bkt.Delete(ctx, metaFile); err != nil {
			return errors.Wrapf(err, "delete %s", metaFile)
		}
		level.Debug(logger).Log("msg", "deleted file", "file", metaFile, "bucket", bkt.Name())
	}

	// Delete the block objects, but skip:
	// - The metaFile as we just deleted. This is required for eventual object storages (list after write).
	// - The deletionMarkFile as we'll delete it at last.
	err = deleteDirRec(ctx, logger, bkt, id.String(), func(name string) bool {
		return name == metaFile || name == deletionMarkFile
	})
	if err != nil {
		return err
	}

	// Delete block deletion mark.
	ok, err = bkt.Exists(ctx, deletionMarkFile)
	if err != nil {
		return errors.Wrapf(err, "stat %s", deletionMarkFile)
	}

	if ok {
		if err := bkt.Delete(ctx, deletionMarkFile); err != nil {
			return errors.Wrapf(err, "delete %s", deletionMarkFile)
		}
		level.Debug(logger).Log("msg", "deleted file", "file", deletionMarkFile, "bucket", bkt.Name())
	}

	return nil
}

// deleteDirRec removes all objects prefixed with dir from the bucket. It skips objects that return true for the passed keep function.
// NOTE: For objects removal use `block.Delete` strictly.
func deleteDirRec(ctx context.Context, logger log.Logger, bkt objstore.Bucket, dir string, keep func(name string) bool) error {
	return bkt.Iter(ctx, dir, func(name string) error {
		// If we hit a directory, call DeleteDir recursively.
		if strings.HasSuffix(name, objstore.DirDelim) {
			return deleteDirRec(ctx, logger, bkt, name, keep)
		}
		if keep(name) {
			return nil
		}
		if err := bkt.Delete(ctx, name); err != nil {
			return err
		}
		level.Debug(logger).Log("msg", "deleted file", "file", name, "bucket", bkt.Name())
		return nil
	})
}
