package arcticdb

import (
	"unsafe"

	"go.uber.org/atomic"
)

type SentinelType uint8

const (
	None SentinelType = iota
	Compacting
	Compacted
)

// Node is a Part that is a part of a linked-list.
type Node struct {
	next *atomic.UnsafePointer
	part *Part

	sentinel SentinelType // sentinel nodes contain no parts, and are to indicate the start of a new sub list
}

type PartList struct {
	next  *atomic.UnsafePointer
	total *atomic.Uint64

	// listType indicates the type of list this list is
	listType SentinelType
}

// NewPartList creates a new PartList using atomic constructs.
func NewPartList(next unsafe.Pointer, total uint64, s SentinelType) *PartList {
	return &PartList{
		next:     atomic.NewUnsafePointer(next),
		total:    atomic.NewUint64(total),
		listType: s,
	}
}

// Sentinel adds a new sentinel node to the list, and returns the sub list starting from that sentinel.
func (l *PartList) Sentinel(s SentinelType) *PartList {
	node := &Node{
		sentinel: s,
	}
	for { // continue until a successful compare and swap occurs
		next := l.next.Load()
		node.next = atomic.NewUnsafePointer(next)
		if l.next.CAS(next, unsafe.Pointer(node)) {
			// TODO should we add sentinels to the total?
			return NewPartList(next, l.total.Inc(), s)
		}
	}
}

// Prepend a node onto the front of the list.
func (l *PartList) Prepend(part *Part) *Node {
	node := &Node{
		part: part,
	}
	for { // continue until a successful compare and swap occurs
		next := l.next.Load()
		node.next = atomic.NewUnsafePointer(next)
		if next != nil && (*Node)(next).sentinel == Compacted { // This list is apart of a compacted granule, propogate the compacted value so each subsequent Prepend can return the correct value
			node.sentinel = Compacted
		}
		if l.next.CAS(next, unsafe.Pointer(node)) {
			l.total.Inc()
			return node
		}
	}
}

// Iterate accesses every node in the list.
func (l *PartList) Iterate(iterate func(*Part) bool) {
	next := l.next.Load()
	for {
		node := (*Node)(next)
		if node == nil {
			return
		}
		switch node.part {
		case nil: // sentinel node
			if l.listType != None && node.sentinel != l.listType { // if we've encountererd a sentinel node from a different type of list we must exit
				return
			}
		default: // normal node
			if !iterate(node.part) { // if the part == nil then this is a sentinel node, and we can skip it
				return
			}
		}
		next = node.next.Load()
	}
}
