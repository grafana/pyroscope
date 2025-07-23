package fsm

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

func TestBoltDB_open_restore(t *testing.T) {
	tempDir := t.TempDir()
	buf := bytes.NewBuffer(nil)
	db := newDB(log.NewLogfmtLogger(buf), newMetrics(nil), Config{DataDir: tempDir})
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
	assert.False(t, strings.Contains(buf.String(), "compacting snapshot"))
}

func TestBoltDB_open_restore_compact(t *testing.T) {
	tempDir := t.TempDir()
	buf := bytes.NewBuffer(nil)
	db := newDB(log.NewLogfmtLogger(buf), newMetrics(nil), Config{
		DataDir:                  tempDir,
		SnapshotCompactOnRestore: true,
	})
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
	assert.True(t, strings.Contains(buf.String(), "compacting snapshot"))
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
