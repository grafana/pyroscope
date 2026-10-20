package model

import (
	"sync"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

type ProfilePresenceMerger struct {
	mu   sync.Mutex
	seen map[string]*queryv1.ProfilePresenceEntry
}

func NewProfilePresenceMerger() *ProfilePresenceMerger {
	return &ProfilePresenceMerger{seen: make(map[string]*queryv1.ProfilePresenceEntry)}
}

func (m *ProfilePresenceMerger) MergeProfilePresence(entries []*queryv1.ProfilePresenceEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range entries {
		m.seen[e.ProfileId] = e
	}
}

func (m *ProfilePresenceMerger) Profiles() []*queryv1.ProfilePresenceEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	profiles := make([]*queryv1.ProfilePresenceEntry, 0, len(m.seen))
	for _, e := range m.seen {
		profiles = append(profiles, e)
	}
	return profiles
}
