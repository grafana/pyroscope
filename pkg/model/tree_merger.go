package model

import (
	"sync"
)

type TreeMerger struct {
	mu sync.Mutex
	t  *FunctionNameTree
}

func NewTreeMerger() *TreeMerger {
	return new(TreeMerger)
}

func (m *TreeMerger) MergeTree(t *FunctionNameTree) {
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
	t, err := UnmarshalTree[FuntionName, FuntionNameI](b)
	if err != nil {
		return err
	}
	m.MergeTree(t)
	return nil
}

func (m *TreeMerger) Tree() *FunctionNameTree {
	if m.t == nil {
		return new(FunctionNameTree)
	}
	return m.t
}

func (m *TreeMerger) IsEmpty() bool {
	return m.t == nil
}
