package settings

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/exp/slices"

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

func (s *memoryStore) Get(ctx context.Context, tenantID string) ([]*settingsv1.Setting, error) {
	s.storeLock.RLock()
	defer s.storeLock.RUnlock()

	tenantSettings := s.store[tenantID]

	settings := make([]*settingsv1.Setting, 0, len(s.store[tenantID]))
	for _, setting := range tenantSettings {
		settings = append(settings, setting)
	}

	slices.SortFunc(settings, func(a, b *settingsv1.Setting) bool {
		return a.Name < b.Name
	})
	return settings, nil
}

func (s *memoryStore) Set(ctx context.Context, tenantID string, setting *settingsv1.Setting) (*settingsv1.Setting, error) {
	s.storeLock.Lock()
	defer s.storeLock.Unlock()

	_, ok := s.store[tenantID]
	if !ok {
		s.store[tenantID] = make(map[string]*settingsv1.Setting, 1)
	}

	oldSetting, ok := s.store[tenantID][setting.Name]
	if ok && oldSetting.ModifiedAt > setting.ModifiedAt {
		return nil, fmt.Errorf("failed to update %s: newer update already written", setting.Name)
	}
	s.store[tenantID][setting.Name] = setting

	return setting, nil
}
