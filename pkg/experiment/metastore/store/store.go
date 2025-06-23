package store

import (
	"bytes"
	"errors"

	"go.etcd.io/bbolt"
)

var ErrNotFound = errors.New("not found")

type KV struct {
	Key   []byte
	Value []byte
}

func NewCursorIter(cursor *bbolt.Cursor) *CursorIterator {
	return &CursorIterator{cursor: cursor}
}

type CursorIterator struct {
	cursor *bbolt.Cursor
	seek   bool
	k, v   []byte

	// Prefix that keys must start with.
	Prefix []byte
	// Keys that start with this prefix will be skipped.
	SkipPrefix []byte
}

func (c *CursorIterator) Next() bool {
	if !c.seek {
		c.k, c.v = c.cursor.Seek(c.Prefix)
		c.seek = true
		return c.valid()
	}
	for {
		c.k, c.v = c.cursor.Next()
		if !c.valid() {
			return false
		}
		if len(c.SkipPrefix) == 0 || !bytes.HasPrefix(c.k, c.SkipPrefix) {
			return true
		}
	}
}

func (c *CursorIterator) valid() bool {
	return c.k != nil && (len(c.Prefix) == 0 || bytes.HasPrefix(c.k, c.Prefix))
}

func (c *CursorIterator) At() KV       { return KV{Key: c.k, Value: c.v} }
func (c *CursorIterator) Err() error   { return nil }
func (c *CursorIterator) Close() error { return nil }
