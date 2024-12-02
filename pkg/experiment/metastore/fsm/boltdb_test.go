package fsm

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/pkg/util"
)

func TestBoltDB_open_restore(t *testing.T) {
	tempDir := t.TempDir()
	db := newDB(util.Logger, newMetrics(nil), tempDir)
	require.NoError(t, db.open(false))

	data := []string{
		"k1", "v1",
		"k2", "v2",
		"k3", "v3",
	}

	snapshotSource := filepath.Join(tempDir, "snapshot_source")
	require.NoError(t, createDB(t, snapshotSource, data).Close())
	s, err := os.ReadFile(snapshotSource)
	require.NoError(t, err)
	require.NoError(t, db.restore(bytes.NewReader(s)))

	collected := make([]string, 0, len(data))
	require.NoError(t, db.boltdb.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("test"))
		assert.NotNil(t, b)
		return b.ForEach(func(k, v []byte) error {
			collected = append(collected, string(k), string(v))
			return nil
		})
	}))

	assert.Equal(t, data, collected)
}

func createDB(t *testing.T, path string, pairs []string) *bbolt.DB {
	opts := bbolt.Options{
		NoGrowSync:     true,
		NoFreelistSync: true,
		NoSync:         true,
		FreelistType:   bbolt.FreelistMapType,
	}
	db, err := bbolt.Open(path, 0644, &opts)
	require.NoError(t, err)
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("test"))
		require.NoError(t, err)
		for len(pairs) > 1 {
			if err = bucket.Put([]byte(pairs[0]), []byte(pairs[1])); err != nil {
				return err
			}
			pairs = pairs[2:]
		}
		return nil
	}))
	return db
}
