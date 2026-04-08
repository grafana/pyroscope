package model

import (
	"sync"
)

type TreeMerger[N NodeName, I NodeNameI[N]] struct {
	mu sync.Mutex
	t  *Tree[N, I]
}

func NewTreeMerger[N NodeName, I NodeNameI[N]]() *TreeMerger[N, I] {
	return new(TreeMerger[N, I])
}

func (m *TreeMerger[N, I]) MergeTree(t *Tree[N, I]) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.t != nil {
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

func (m *TreeMerger[N, I]) MergeTreeBytes(b []byte, opts ...TreeMergeOption[N]) error {
	var o = new(treeMergeOption[N])
	for _, f := range opts {
		f(o)
	}

	// TODO(kolesnikovae): Ideally, we should not have
	// the intermediate tree t but update m.t reading
	// raw bytes b directly.
	t, err := UnmarshalTree[N, I](b)
	if err != nil {
		return err
	}
	if o.formatNodeNames != nil {
		t.FormatNodeNames(o.formatNodeNames)
	}
	m.MergeTree(t)
	return nil
}

func (m *TreeMerger[N, I]) Tree() *Tree[N, I] {
	if m.t == nil {
		return new(Tree[N, I])
	}
	return m.t
}

func (m *TreeMerger[N, I]) IsEmpty() bool {
	return m.t == nil
}
