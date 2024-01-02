// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/thanos-io/thanos/blob/main/pkg/block/block_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Thanos Authors.

package block_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
	"go.uber.org/goleak"

	objstore_testutil "github.com/grafana/pyroscope/pkg/objstore/testutil"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	block_testutil "github.com/grafana/pyroscope/pkg/phlaredb/block/testutil"
	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
	"github.com/grafana/pyroscope/pkg/test"
)

func TestIsBlockDir(t *testing.T) {
	for _, tc := range []struct {
		input string
		id    ulid.ULID
		bdir  bool
	}{
		{
			input: "",
			bdir:  false,
		},
		{
			input: "something",
			bdir:  false,
		},
		{
			id:    ulid.MustNew(1, nil),
			input: ulid.MustNew(1, nil).String(),
			bdir:  true,
		},
		{
			id:    ulid.MustNew(2, nil),
			input: "/" + ulid.MustNew(2, nil).String(),
			bdir:  true,
		},
		{
			id:    ulid.MustNew(3, nil),
			input: "some/path/" + ulid.MustNew(3, nil).String(),
			bdir:  true,
		},
		{
			input: ulid.MustNew(4, nil).String() + "/something",
			bdir:  false,
		},
	} {
		t.Run(tc.input, func(t *testing.T) {
			id, ok := block.IsBlockDir(tc.input)
			require.Equal(t, tc.bdir, ok)

			if id.Compare(tc.id) != 0 {
				t.Errorf("expected %s got %s", tc.id, id)
				t.FailNow()
			}
		})
	}
}

func TestDelete(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	ctx := context.Background()

	runTest := func(t *testing.T, bkt objstore.Bucket) {
		{
			meta, dir := block_testutil.CreateBlock(t, func() []*testhelper.ProfileBuilder {
				return []*testhelper.ProfileBuilder{
					testhelper.NewProfileBuilder(int64(1)).
						CPUProfile().
						WithLabels(
							"job", "a",
						).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
				}
			})

			require.NoError(t, block.Upload(ctx, log.NewNopLogger(), bkt, path.Join(dir, meta.ULID.String())))
			require.Equal(t, 9, len(objects(t, bkt, meta.ULID)))

			markedForDeletion := promauto.With(prometheus.NewRegistry()).NewCounter(prometheus.CounterOpts{Name: "test"})
			require.NoError(t, block.MarkForDeletion(ctx, log.NewNopLogger(), bkt, meta.ULID, "", false, markedForDeletion))

			// Full delete.
			require.NoError(t, block.Delete(ctx, log.NewNopLogger(), bkt, meta.ULID))
			require.Equal(t, 0, len(objects(t, bkt, meta.ULID)))
		}
		{
			b2, tmpDir := block_testutil.CreateBlock(t, func() []*testhelper.ProfileBuilder {
				return []*testhelper.ProfileBuilder{
					testhelper.NewProfileBuilder(int64(1)).
						CPUProfile().
						WithLabels(
							"job", "a",
						).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
				}
			})
			require.NoError(t, block.Upload(ctx, log.NewNopLogger(), bkt, path.Join(tmpDir, b2.ULID.String())))
			require.Equal(t, 9, len(objects(t, bkt, b2.ULID)))

			// Remove meta.json and check if delete can delete it.
			require.NoError(t, bkt.Delete(ctx, path.Join(b2.ULID.String(), block.MetaFilename)))
			require.NoError(t, block.Delete(ctx, log.NewNopLogger(), bkt, b2.ULID))
			require.Equal(t, 0, len(objects(t, bkt, b2.ULID)))
		}
	}

	t.Run(t.Name()+"_inmemory", func(t *testing.T) {
		bkt := objstore.NewInMemBucket()
		runTest(t, bkt)
	})

	t.Run(t.Name()+"_filesystem", func(t *testing.T) {
		bkt, _ := objstore_testutil.NewFilesystemBucket(t, context.Background(), t.TempDir())
		runTest(t, bkt)
	})
}

func objects(t *testing.T, bkt objstore.Bucket, id ulid.ULID) (objects []string) {
	t.Helper()
	require.NoError(t,
		bkt.Iter(context.Background(), id.String(), func(name string) error {
			if strings.HasSuffix(name, objstore.DirDelim) {
				return nil
			}
			objects = append(objects, name)
			return nil
		}, objstore.WithRecursiveIter))
	return
}

func TestUpload(t *testing.T) {
	ctx := context.Background()

	bkt := objstore.NewInMemBucket()
	b1, tmpDir := block_testutil.CreateBlock(t, func() []*testhelper.ProfileBuilder {
		return []*testhelper.ProfileBuilder{
			testhelper.NewProfileBuilder(int64(1)).
				CPUProfile().
				WithLabels(
					"a", "3", "b", "1",
				).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
		}
	})

	require.NoError(t, os.MkdirAll(path.Join(tmpDir, "test", b1.ULID.String()), os.ModePerm))

	t.Run("wrong dir", func(t *testing.T) {
		// Wrong dir.
		err := block.Upload(ctx, log.NewNopLogger(), bkt, path.Join(tmpDir, "not-existing"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "/not-existing: no such file or directory")
	})

	t.Run("wrong existing dir (not a block)", func(t *testing.T) {
		err := block.Upload(ctx, log.NewNopLogger(), bkt, path.Join(tmpDir, "test"))
		require.EqualError(t, err, "not a block dir: ulid: bad data size when unmarshaling")
	})

	t.Run("empty block dir", func(t *testing.T) {
		err := block.Upload(ctx, log.NewNopLogger(), bkt, path.Join(tmpDir, "test", b1.ULID.String()))
		require.Error(t, err)
		require.Contains(t, err.Error(), "/meta.json: no such file or directory")
	})

	t.Run("missing meta.json file", func(t *testing.T) {
		test.Copy(t, path.Join(tmpDir, b1.ULID.String(), block.IndexFilename), path.Join(tmpDir, "test", b1.ULID.String(), block.IndexFilename))

		// Missing meta.json file.
		err := block.Upload(ctx, log.NewNopLogger(), bkt, path.Join(tmpDir, "test", b1.ULID.String()))
		require.Error(t, err)
		require.Contains(t, err.Error(), "/meta.json: no such file or directory")
	})

	test.Copy(t, path.Join(tmpDir, b1.ULID.String(), block.MetaFilename), path.Join(tmpDir, "test", b1.ULID.String(), block.MetaFilename))

	t.Run("full block", func(t *testing.T) {
		require.NoError(t, block.Upload(ctx, log.NewNopLogger(), bkt, path.Join(tmpDir, b1.ULID.String())))
		require.Equal(t, 9, len(bkt.Objects()))
		objs := bkt.Objects()
		require.Contains(t, objs, path.Join(b1.ULID.String(), block.MetaFilename))
		require.Contains(t, objs, path.Join(b1.ULID.String(), block.IndexFilename))
		require.Contains(t, objs, path.Join(b1.ULID.String(), "profiles.parquet"))
	})

	t.Run("upload is idempotent", func(t *testing.T) {
		require.NoError(t, block.Upload(ctx, log.NewNopLogger(), bkt, path.Join(tmpDir, b1.ULID.String())))
		require.Equal(t, 9, len(bkt.Objects()))
		objs := bkt.Objects()
		require.Contains(t, objs, path.Join(b1.ULID.String(), block.MetaFilename))
		require.Contains(t, objs, path.Join(b1.ULID.String(), block.IndexFilename))
		require.Contains(t, objs, path.Join(b1.ULID.String(), "profiles.parquet"))
	})
}

func TestMarkForDeletion(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	ctx := context.Background()

	for _, tcase := range []struct {
		name      string
		preUpload func(t testing.TB, id ulid.ULID, bkt objstore.Bucket)

		blocksMarked int
	}{
		{
			name:         "block marked for deletion",
			preUpload:    func(t testing.TB, id ulid.ULID, bkt objstore.Bucket) {},
			blocksMarked: 1,
		},
		{
			name: "block with deletion mark already, expected log and no metric increment",
			preUpload: func(t testing.TB, id ulid.ULID, bkt objstore.Bucket) {
				deletionMark, err := json.Marshal(block.DeletionMark{
					ID:           id,
					DeletionTime: time.Now().Unix(),
					Version:      block.DeletionMarkVersion1,
				})
				require.NoError(t, err)
				require.NoError(t, bkt.Upload(ctx, path.Join(id.String(), block.DeletionMarkFilename), bytes.NewReader(deletionMark)))
			},
			blocksMarked: 0,
		},
	} {
		t.Run(tcase.name, func(t *testing.T) {
			bkt := objstore.NewInMemBucket()
			b1, tmpDir := block_testutil.CreateBlock(t, func() []*testhelper.ProfileBuilder {
				return []*testhelper.ProfileBuilder{
					testhelper.NewProfileBuilder(int64(1)).
						CPUProfile().
						WithLabels(
							"a", "3", "b", "1",
						).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
				}
			})
			id := b1.ULID

			tcase.preUpload(t, id, bkt)

			require.NoError(t, block.Upload(ctx, log.NewNopLogger(), bkt, path.Join(tmpDir, id.String())))

			c := promauto.With(nil).NewCounter(prometheus.CounterOpts{})
			err := block.MarkForDeletion(ctx, log.NewNopLogger(), bkt, id, "", false, c)
			require.NoError(t, err)
			require.Equal(t, float64(tcase.blocksMarked), promtest.ToFloat64(c))
		})
	}
}

func TestMarkForNoCompact(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	ctx := context.Background()

	for _, tcase := range []struct {
		name      string
		preUpload func(t testing.TB, id ulid.ULID, bkt objstore.Bucket)

		blocksMarked int
	}{
		{
			name:         "block marked",
			preUpload:    func(t testing.TB, id ulid.ULID, bkt objstore.Bucket) {},
			blocksMarked: 1,
		},
		{
			name: "block with no-compact mark already, expected log and no metric increment",
			preUpload: func(t testing.TB, id ulid.ULID, bkt objstore.Bucket) {
				m, err := json.Marshal(block.NoCompactMark{
					ID:            id,
					NoCompactTime: time.Now().Unix(),
					Version:       block.NoCompactMarkVersion1,
				})
				require.NoError(t, err)
				require.NoError(t, bkt.Upload(ctx, path.Join(id.String(), block.NoCompactMarkFilename), bytes.NewReader(m)))
			},
			blocksMarked: 0,
		},
	} {
		t.Run(tcase.name, func(t *testing.T) {
			bkt := objstore.NewInMemBucket()
			meta, tmpDir := block_testutil.CreateBlock(t, func() []*testhelper.ProfileBuilder {
				return []*testhelper.ProfileBuilder{
					testhelper.NewProfileBuilder(int64(1)).
						CPUProfile().
						WithLabels(
							"a", "3", "b", "1",
						).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
				}
			})
			id := meta.ULID

			tcase.preUpload(t, id, bkt)

			require.NoError(t, block.Upload(ctx, log.NewNopLogger(), bkt, path.Join(tmpDir, id.String())))

			c := promauto.With(nil).NewCounter(prometheus.CounterOpts{})
			err := block.MarkForNoCompact(ctx, log.NewNopLogger(), bkt, id, block.ManualNoCompactReason, "", c)
			require.NoError(t, err)
			require.Equal(t, float64(tcase.blocksMarked), promtest.ToFloat64(c))
		})
	}
}

func TestUploadCleanup(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()

	bkt := objstore.NewInMemBucket()
	meta, tmpDir := block_testutil.CreateBlock(t, func() []*testhelper.ProfileBuilder {
		return []*testhelper.ProfileBuilder{
			testhelper.NewProfileBuilder(int64(1)).
				CPUProfile().
				WithLabels(
					"a", "3", "b", "1",
				).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
		}
	})
	b1 := meta.ULID

	{
		errBkt := errBucket{Bucket: bkt, failSuffix: "/index.tsdb"}

		uploadErr := block.Upload(ctx, log.NewNopLogger(), errBkt, path.Join(tmpDir, b1.String()))
		require.ErrorIs(t, uploadErr, errUploadFailed)

		// If upload of index fails, block is deleted.
		require.Equal(t, 0, len(bkt.Objects()))
	}

	{
		errBkt := errBucket{Bucket: bkt, failSuffix: "/meta.json"}

		uploadErr := block.Upload(ctx, log.NewNopLogger(), errBkt, path.Join(tmpDir, b1.String()))
		require.ErrorIs(t, uploadErr, errUploadFailed)

		// If upload of meta.json fails, nothing is cleaned up.
		require.Equal(t, 9, len(bkt.Objects()))
		require.Greater(t, len(bkt.Objects()[path.Join(b1.String(), block.IndexFilename)]), 0)
		require.Greater(t, len(bkt.Objects()[path.Join(b1.String(), block.MetaFilename)]), 0)
	}
}

var errUploadFailed = errors.New("upload failed")

type errBucket struct {
	objstore.Bucket

	failSuffix string
}

func (eb errBucket) Upload(ctx context.Context, name string, r io.Reader) error {
	err := eb.Bucket.Upload(ctx, name, r)
	if err != nil {
		return err
	}

	if strings.HasSuffix(name, eb.failSuffix) {
		return errUploadFailed
	}
	return nil
}
