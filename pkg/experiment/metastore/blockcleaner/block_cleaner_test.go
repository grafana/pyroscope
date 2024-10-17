package blockcleaner

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/oklog/ulid"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/pkg/objstore/providers/memory"
	"github.com/grafana/pyroscope/pkg/util"
)

func Test_AddAndCheck(t *testing.T) {
	db := createDb(t)
	cleaner := newBlockCleaner(func() *bbolt.DB {
		return db
	}, util.Logger, &Config{CompactedBlocksCleanupDelay: model.Duration(time.Second * 2)}, memory.NewInMemBucket())

	blockId := ulid.MustNew(ulid.Now(), rand.Reader).String()
	err := cleaner.AddBlock(0, "tenant", blockId, 1000)
	require.NoError(t, err)

	require.True(t, cleaner.IsRemoved(blockId))
}

func Test_AddAndRemove(t *testing.T) {
	db := createDb(t)
	cleaner := newBlockCleaner(func() *bbolt.DB {
		return db
	}, util.Logger, &Config{CompactedBlocksCleanupDelay: model.Duration(time.Second * 2)}, memory.NewInMemBucket())
	cleaner.isLeader = true

	blockId := ulid.MustNew(ulid.Now(), rand.Reader).String()
	err := cleaner.AddBlock(0, "tenant", blockId, 1000)
	require.NoError(t, err)
	err = cleaner.bkt.Upload(context.Background(), fmt.Sprintf("blocks/0/tenant/%s", blockId), bytes.NewReader([]byte{1, 2, 3}))
	require.NoError(t, err)

	cleaner.doCleanup(5000)
	require.False(t, cleaner.IsRemoved(blockId))
	inBucket, err := cleaner.bkt.Exists(context.Background(), fmt.Sprintf("blocks/0/tenant/%s", blockId))
	require.NoError(t, err)
	require.False(t, inBucket)
}

func createDb(t *testing.T) *bbolt.DB {
	opts := *bbolt.DefaultOptions
	opts.ReadOnly = false
	opts.NoSync = true
	db, err := bbolt.Open(filepath.Join(t.TempDir(), "db.boltdb"), 0644, &opts)
	require.NoError(t, err)
	return db
}
