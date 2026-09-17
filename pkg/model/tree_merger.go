package model

import (
	"sync"
)

type TreeMerger[N NodeName, I NodeNameI[N]] struct {
	mu sync.Mutex
	t  *Tree[N, I]
	// Totals of t are deferred while merging serialized trees and
	// recomputed on access: see finalize. nodes is a lower bound of the
	// tree size (the largest single merged tree), used as a size hint.
	dirty bool
	nodes int
}

func NewTreeMerger[N NodeName, I NodeNameI[N]]() *TreeMerger[N, I] {
	return new(TreeMerger[N, I])
}

func (m *TreeMerger[N, I]) MergeTree(t *Tree[N, I]) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.t != nil {
		// Tree.Merge reads totals of both trees.
		m.finalize()
		m.t.Merge(t)
	} else {
		m.t = t
	}
}

type treeMergeOption[N any] struct {
	formatNodeNames func(N) N
}

type TreeMergeOption[N any] func(o *treeMergeOption[N])

func WithTreeMergeFormatNodeNames[N any](f func(N) N) TreeMergeOption[N] {
	return func(o *treeMergeOption[N]) {
		o.formatNodeNames = f
	}
}

// MergeTreeBytes merges a marshaled tree into the merger's tree, reading
// the raw bytes directly. On a malformed input an error is returned and
// the merger's tree may include a partially merged prefix of the input.
func (m *TreeMerger[N, I]) MergeTreeBytes(b []byte, opts ...TreeMergeOption[N]) error {
	var o = new(treeMergeOption[N])
	for _, f := range opts {
		f(o)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.t == nil {
		m.t = new(Tree[N, I])
	}
	records, err := m.t.mergeBytes(b, o.formatNodeNames)
	if err != nil {
		return err
	}
	m.dirty = true
	m.nodes = max(m.nodes, records)
	return nil
}

func (m *TreeMerger[N, I]) Tree() *Tree[N, I] {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.t == nil {
		return new(Tree[N, I])
	}
	m.finalize()
	return m.t
}

func (m *TreeMerger[N, I]) finalize() {
	if m.dirty {
		m.t.recomputeTotals(m.nodes)
		m.dirty = false
	}
}

func (m *TreeMerger[N, I]) IsEmpty() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.t == nil
}
