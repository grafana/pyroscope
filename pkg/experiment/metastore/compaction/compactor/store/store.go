package store

import (
	"bytes"
	"errors"

	"go.etcd.io/bbolt"
)

var ErrorNotFound = errors.New("not found")

type kv struct {
	key   []byte
	value []byte
}

func newCursorIter(prefix []byte, cursor *bbolt.Cursor) *cursorIterator {
	return &cursorIterator{prefix: prefix, cursor: cursor}
}

type cursorIterator struct {
	cursor *bbolt.Cursor
	seek   bool
	prefix []byte
	k, v   []byte
}

func (c *cursorIterator) Next() bool {
	if !c.seek {
		c.k, c.v = c.cursor.Seek(c.prefix)
		c.seek = true
	} else {
		c.k, c.v = c.cursor.Next()
	}
	return c.valid()
}

func (c *cursorIterator) valid() bool {
	return c.k != nil && (len(c.prefix) == 0 || bytes.HasPrefix(c.k, c.prefix))
}

func (c *cursorIterator) At() kv       { return kv{key: c.k, value: c.v} }
func (c *cursorIterator) Err() error   { return nil }
func (c *cursorIterator) Close() error { return nil }
