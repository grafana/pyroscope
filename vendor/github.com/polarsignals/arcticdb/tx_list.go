package arcticdb

import (
	"time"
	"unsafe"

	"go.uber.org/atomic"
)

type TxNode struct {
	next *atomic.UnsafePointer
	tx   uint64
}

type TxPool struct {
	next  *atomic.UnsafePointer
	drain chan interface{}
}

// NewTxPool returns a new TxPool and starts the pool cleaner routine.
func NewTxPool(watermark *atomic.Uint64) *TxPool {
	txpool := &TxPool{
		next:  atomic.NewUnsafePointer(unsafe.Pointer(nil)),
		drain: make(chan interface{}, 1),
	}
	go txpool.cleaner(watermark)
	return txpool
}

// Prepend a node onto the front of the list.
func (l *TxPool) Prepend(tx uint64) *TxNode {
	node := &TxNode{
		tx: tx,
	}
	for { // continue until a successful compare and swap occurs.
		next := l.next.Load()
		node.next = atomic.NewUnsafePointer(next)
		if l.next.CAS(next, unsafe.Pointer(node)) {
			select {
			case l.drain <- true:
				return node
			default:
				return node
			}
		}
	}
}

// Iterate accesses every node in the list.
func (l *TxPool) Iterate(iterate func(tx uint64) bool) {
	next := l.next.Load()
	prev := unsafe.Pointer(nil)
	for {
		node := (*TxNode)(next)
		if node == nil {
			return
		}
		if iterate(node.tx) {
			if prev == nil { // we're removing the first node
				l.next.CAS(nil, node.next.Load())
			} else {
				// set the previous nodes next to this nodes nex
				prevnode := (*TxNode)(prev)
				prevnode.next.CAS(prevnode.next.Load(), node.next.Load())
			}
		}
		prev = next
		next = node.next.Load()
	}
}

// cleaner sweeps the pool periodically, and bubbles up the given watermark.
// this function does not return.
func (l *TxPool) cleaner(watermark *atomic.Uint64) {
	ticker := time.NewTicker(time.Millisecond * 10)
	defer ticker.Stop()

	for {
		select { // sweep whenever notified or when ticker
		case <-l.drain:
			l.sweep(watermark)
		case <-ticker.C:
			l.sweep(watermark)
		}
	}
}

func (l *TxPool) sweep(watermark *atomic.Uint64) {
	l.Iterate(func(tx uint64) bool {
		mark := watermark.Load()
		switch {
		case mark+1 == tx:
			watermark.Inc()
			return true // return true to indicate that this node should be removed from the tx list.
		case mark >= tx:
			return true
		default:
			return false
		}
	})
}
