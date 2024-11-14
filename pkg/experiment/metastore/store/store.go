package store

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

var ErrorNotFound = errors.New("not found")

type KV struct {
	Key   []byte
	Value []byte
}

func NewCursorIter(prefix []byte, cursor *bbolt.Cursor) *CursorIterator {
	return &CursorIterator{prefix: prefix, cursor: cursor}
}

type CursorIterator struct {
	cursor *bbolt.Cursor
	seek   bool
	prefix []byte
	k, v   []byte
}

func (c *CursorIterator) Next() bool {
	if !c.seek {
		c.k, c.v = c.cursor.Seek(c.prefix)
		c.seek = true
	} else {
		c.k, c.v = c.cursor.Next()
	}
	return c.valid()
}

func (c *CursorIterator) valid() bool {
	return c.k != nil && (len(c.prefix) == 0 || bytes.HasPrefix(c.k, c.prefix))
}

func (c *CursorIterator) At() KV       { return KV{Key: c.k, Value: c.v} }
func (c *CursorIterator) Err() error   { return nil }
func (c *CursorIterator) Close() error { return nil }

func TestDB(t *testing.T) *bbolt.DB {
	tempDir := t.TempDir()
	opts := bbolt.Options{
		NoGrowSync:      true,
		NoFreelistSync:  true,
		FreelistType:    bbolt.FreelistMapType,
		InitialMmapSize: 32 << 20,
		NoSync:          true,
	}
	db, err := bbolt.Open(filepath.Join(tempDir, "boltdb"), 0644, &opts)
	require.NoError(t, err)
	return db
}
