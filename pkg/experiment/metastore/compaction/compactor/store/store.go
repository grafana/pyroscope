package store

import (
	"bytes"
	"errors"

	"go.etcd.io/bbolt"
)

var ErrorNotFound = errors.New("not found")

type KeyPrefix string

func (p KeyPrefix) Key(name string) []byte { return append([]byte(p), name...) }
func (p KeyPrefix) Matches(k []byte) bool  { return bytes.HasPrefix(k, []byte(p)) }

const (
	JobStateKeyPrefix KeyPrefix = "cs:"
	JobPlanKeyPrefix  KeyPrefix = "cp:"
)

func newCursorIter(prefix KeyPrefix, cursor *bbolt.Cursor) *cursorIterator {
	iter := cursorIterator{prefix: prefix, cursor: cursor}
	iter.k, iter.v = cursor.Seek([]byte(prefix))
	return &iter
}

type cursorIterator struct {
	prefix KeyPrefix
	cursor *bbolt.Cursor
	k, v   []byte
}

func (c *cursorIterator) Next() bool {
	if c.prefix.Matches(c.k) {
		c.k, c.v = c.cursor.Next()
		return c.k != nil
	}
	return false
}

func (c *cursorIterator) At() []byte   { return c.v }
func (c *cursorIterator) Err() error   { return nil }
func (c *cursorIterator) Close() error { return nil }
