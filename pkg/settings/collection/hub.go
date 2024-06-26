package collection

import (
	"sync"

	"github.com/cespare/xxhash/v2"
	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
)

const defaultScope = "profiles-collection"

type topic struct {
	name    string
	ch      chan struct{}
	update  func() ([]byte, error)
	err     error
	content []byte
	hasher  xxhash.Digest
	hash    uint64
	clients map[*Client]uint64
}

func newTopic(name string, f func() ([]byte, error)) *topic {
	return &topic{
		name:    name,
		ch:      make(chan struct{}),
		update:  f,
		hash:    0,
		clients: make(map[*Client]uint64),
	}
}

func (t *topic) get() {
	t.content, t.err = t.update()
	if t.err != nil {
		return
	}

	t.hasher.Reset()
	_, t.err = t.hasher.Write(t.content)
	if t.err != nil {
		return
	}
	t.hash = t.hasher.Sum64()
}

type hubKey struct {
	tenantID string
	scope    string
}

type Hub struct {
	lck        sync.RWMutex
	nextRuleID int64
	rules      []*settingsv1.CollectionRule

	app *Collection

	topics map[string]*topic

	agentsPublishing       map[*Client]bool // are particular grafana agents publishing their targets
	agentsPublishingActive bool             // is agent publishing requested
	agents                 map[string]*settingsv1.CollectionInstance

	// Register requests from the clients.
	registerCh chan *Client

	// Unregister requests from clients.
	unregisterCh chan *Client

	// Update agent targets
	agentCh chan *settingsv1.CollectionInstance

	// Update rules
	rulesCh chan func(*Hub)
}

func (a *Hub) insertRule(data *settingsv1.CollectionPayloadRuleInsert) {
	a.lck.Lock()
	defer a.lck.Unlock()

	if a.nextRuleID == 0 {
		for _, r := range a.rules {
			if r.Id > a.nextRuleID {
				a.nextRuleID = r.Id
			}
		}
		a.nextRuleID++
	}

	pos := 0
	// find position to insert
	if data.After != nil {
		after := *data.After
		for i, r := range a.rules {
			if r.Id == after {
				pos = i + 1
				break
			}
		}
	}

	// overwrite id
	rule := data.Rule.CloneVT()
	rule.Id = a.nextRuleID
	a.nextRuleID++

	// insert rule to correct position
	a.rules = append(a.rules, nil)
	copy(a.rules[pos+1:], a.rules[pos:])
	a.rules[pos] = rule

}

func (h *Hub) deleteRule(id int64) {
	h.lck.Lock()
	defer h.lck.Unlock()

	for i, r := range h.rules {
		if r.Id == id {
			h.rules = append(h.rules[:i], h.rules[i+1:]...)
			break
		}
	}
}
