package store

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/v2/pkg/test"
)

func TestCursorIterator_SkipPrefixOnInitialSeek(t *testing.T) {
	db := test.BoltDB(t)
	bucketName := []byte("b")

	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucket(bucketName)
		if err != nil {
			return err
		}
		if _, err := b.CreateBucket([]byte(".strings")); err != nil {
			return err
		}
		if err := b.Put([]byte(".index"), []byte("idx")); err != nil {
			return err
		}
		if err := b.Put([]byte("01J9E3ZXCA1QNSSZY8Z7A09VGX"), []byte("v1")); err != nil {
			return err
		}
		return b.Put([]byte("01J9E3ZXCA1QNSSZY8Z7A09VGY"), []byte("v2"))
	}))

	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		it := NewCursorIter(tx.Bucket(bucketName).Cursor())
		it.SkipPrefix = []byte{'.'}

		var keys []string
		for it.Next() {
			keys = append(keys, string(it.At().Key))
		}
		require.NoError(t, it.Err())
		require.NoError(t, it.Close())
		require.Equal(t, []string{
			"01J9E3ZXCA1QNSSZY8Z7A09VGX",
			"01J9E3ZXCA1QNSSZY8Z7A09VGY",
		}, keys)
		return nil
	}))
}
