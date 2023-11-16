package settings

import (
	"context"
	"sync"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
)

func NewMemoryStore() (Store, error) {
	store := &memoryStore{
		store: make(map[string]map[string]*settingsv1.Setting),
	}
	return store, nil
}

type memoryStore struct {
	storeLock sync.RWMutex

	// store is kv pairs, indexed first by tenant id.
	store map[string]map[string]*settingsv1.Setting
}

func (s *memoryStore) All(ctx context.Context, tenantIDs ...string) ([]*settingsv1.Setting, error) {
	s.storeLock.RLock()
	defer s.storeLock.RUnlock()

	length := 0
	for _, id := range tenantIDs {
		length += len(s.store[id])
	}

	// TODO(bryan): deduplicate
	settings := make([]*settingsv1.Setting, 0, length)
	for _, id := range tenantIDs {
		for _, setting := range s.store[id] {
			settings = append(settings, setting)
		}
	}
	return settings, nil
}

func (s *memoryStore) Set(ctx context.Context, setting *settingsv1.Setting, tenantIDs ...string) (*settingsv1.Setting, error) {
	s.storeLock.Lock()
	defer s.storeLock.Unlock()

	for _, id := range tenantIDs {
		_, ok := s.store[id]
		if !ok {
			s.store[id] = make(map[string]*settingsv1.Setting, 1)
		}
		s.store[id][setting.Name] = setting
	}
	return setting, nil
}
