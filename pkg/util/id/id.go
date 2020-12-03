package id

import "sync/atomic"

type ID int64

func (g *ID) Next() int64 {
	return atomic.AddInt64((*int64)(g), 1)
}
