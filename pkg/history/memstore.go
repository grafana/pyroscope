package history

import "context"

type MemStoreManager struct {
	store map[QueryID]*Entry
}

func NewMemStoreManager() *MemStoreManager {
	return &MemStoreManager{
		store: make(map[QueryID]*Entry),
	}
}

func (m *MemStoreManager) Add(_ context.Context, e *Entry) (QueryID, error) {
	qid := GenerateQueryID()
	m.store[qid] = e
	return qid, nil
}

func (m *MemStoreManager) Get(_ context.Context, qid QueryID) (*Entry, error) {
	return m.store[qid], nil
}

func (*MemStoreManager) List(_ context.Context, _ string) ([]*Entry, string, error) {
	return []*Entry{}, "", nil
}
