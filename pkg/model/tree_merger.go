package model

import (
	"sync"
)

type TreeMerger struct {
	mu sync.Mutex
	t  *Tree
}

func NewTreeMerger() *TreeMerger {
	return new(TreeMerger)
}

func (m *TreeMerger) MergeTree(t *Tree) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.t != nil {
		m.t.Merge(t)
	} else {
		m.t = t
	}
}

func (m *TreeMerger) MergeTreeBytes(b []byte) error {
	// TODO(kolesnikovae): Ideally, we should not have
	// the intermediate tree t but update m.t reading
	// raw bytes b directly.
	t, err := UnmarshalTree(b)
	if err != nil {
		return err
	}
	m.MergeTree(t)
	return nil
}

func (m *TreeMerger) Tree() *Tree {
	if m.t == nil {
		return new(Tree)
	}
	return m.t
}

func (m *TreeMerger) IsEmpty() bool {
	return m.t == nil
}
