package collection

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
)

const defaultScope = "profiles-collection"

type topic struct {
	name    string
	ch      chan struct{}
	update  func(*settingsv1.CollectionPayloadData) error
	err     error
	content *settingsv1.CollectionPayloadData
	hash    uint64
	clients map[*client]uint64
}

func newTopic(name string, f func(*settingsv1.CollectionPayloadData) error) *topic {
	return &topic{
		name:    name,
		ch:      make(chan struct{}),
		update:  f,
		hash:    0,
		clients: make(map[*client]uint64),
		content: &settingsv1.CollectionPayloadData{},
	}
}

type buf struct {
	data   []byte
	hasher xxhash.Digest
}

func (t *topic) get(b *buf) {
	t.err = t.update(t.content)
	if t.err != nil {
		return
	}

	n := t.content.SizeVT()
	if cap(b.data) < n {
		b.data = make([]byte, n)
	} else {
		b.data = b.data[:n]
	}
	_, t.err = t.content.MarshalToSizedBufferVT(b.data)
	if t.err != nil {
		return
	}

	b.hasher.Reset()
	_, t.err = b.hasher.Write(b.data)
	if t.err != nil {
		return
	}
	t.hash = b.hasher.Sum64()
}

type hubKey struct {
	tenantID string
	scope    string
}

type hub struct {
	logger     log.Logger
	lck        sync.RWMutex
	nextRuleID int64
	rules      []*settingsv1.CollectionRule

	topics map[string]*topic

	clients map[*client]struct{}

	agentsPublishing       map[*client]bool // are particular grafana agents publishing their targets
	agentsPublishingActive bool             // is agent publishing requested
	agents                 map[string]*settingsv1.CollectionInstance

	// Register requests from the clients.
	registerCh chan *client

	// Unregister requests from clients.
	unregisterCh chan *client

	// Update agent targets
	agentCh chan *settingsv1.CollectionInstance

	// Update rules
	rulesCh chan func(*hub)

	// keep data and hash buffer around
	buf buf
}

func newHub(logger log.Logger, topicsF func(*hub) []*topic) *hub {
	h := &hub{
		logger:           logger,
		topics:           make(map[string]*topic),
		clients:          make(map[*client]struct{}),
		agentsPublishing: make(map[*client]bool),
		agents:           make(map[string]*settingsv1.CollectionInstance),
		agentCh:          make(chan *settingsv1.CollectionInstance),
		registerCh:       make(chan *client),
		unregisterCh:     make(chan *client),
		rulesCh:          make(chan func(*hub), 32),
	}

	for _, topic := range topicsF(h) {
		h.topics[topic.name] = topic
	}

	return h
}

// check data for topics to sent
func (h *hub) updateClientTopicsToPublish(client *client) error {
	var payload []*settingsv1.CollectionPayloadData
	for _, t := range h.topics {
		clientHash, ok := t.clients[client]
		if !ok {
			continue
		}

		if t.hash == 0 {
			t.get(&h.buf)
		}

		if clientHash == t.hash {
			continue
		}

		// need update
		payload = append(payload, t.content)
		t.clients[client] = t.hash
	}

	if len(payload) > 0 {
		merged := mergePayloads(payload...)
		data, err := json.Marshal(&settingsv1.CollectionMessage{
			PayloadData: merged,
		})
		if err != nil {
			return fmt.Errorf("error generation JSON: %w", err)
		}
		client.send <- data
	}
	return nil
}

// check if agent needs toggle publishing
func (h *hub) updateClientToggleAgentSubscription(client *client) error {
	active, ok := h.agentsPublishing[client]
	if !ok {
		return nil
	}
	if active != h.agentsPublishingActive {

		msg := settingsv1.CollectionMessage{
			PayloadSubscribe: &settingsv1.CollectionPayloadSubscribe{},
		}
		if !active {
			msg.PayloadSubscribe.Topics = []string{"agents"}
		}

		data, err := json.Marshal(&msg)
		if err != nil {
			return err
		}

		level.Debug(client.logger).Log("request agent publishing", "client", fmt.Sprintf("%p", client), "data", string(data))
		client.send <- data

		h.agentsPublishing[client] = h.agentsPublishingActive
	}
	return nil
}

func (h *hub) updateClient(client *client) {
	if err := h.updateClientTopicsToPublish(client); err != nil {
		level.Error(client.logger).Log("msg", "error updating client topics to publish", "err", err)
	}
	if err := h.updateClientToggleAgentSubscription(client); err != nil {
		level.Error(client.logger).Log("msg", "error updating client to subscribe to agents", "err", err)
	}
}

func (h *hub) updateAgents() {
	t, ok := h.topics["agents"]
	if !ok {
		return
	}
	t.get(&h.buf)

	// update frontend with new agents data
	for client := range t.clients {
		h.updateClient(client)
	}
}

func (a *hub) insertRule(data *settingsv1.CollectionPayloadRuleInsert) {
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

func (h *hub) deleteRule(id int64) {
	h.lck.Lock()
	defer h.lck.Unlock()

	for i, r := range h.rules {
		if r.Id == id {
			h.rules = append(h.rules[:i], h.rules[i+1:]...)
			break
		}
	}
}

func (h *hub) run(stopCh <-chan struct{}) {
	for {
		select {
		case client := <-h.registerCh:
			h.clients[client] = struct{}{}
			for _, topic := range client.subscribedTopics {
				t, ok := h.topics[topic]
				if !ok {
					continue
				}
				_, ok = t.clients[client]
				if ok {
					continue
				}
				t.clients[client] = 0

			}
			if client.isRuleManager() {
				h.agentsPublishing[client] = false
			}

			// check if agents need toggle publishing
			agentsTopic, ok := h.topics["agents"]
			agentsPublishingRequested := false
			if ok {
				agentsPublishingRequested = len(agentsTopic.clients) != 0
			}
			if agentsPublishingRequested != h.agentsPublishingActive {
				if agentsPublishingRequested {
					slog.Debug("agents publishing has been enabled")
				} else {
					slog.Debug("agents publishing has been disabled")
				}
				h.agentsPublishingActive = agentsPublishingRequested
				// send message to all agents
				for agent := range h.agentsPublishing {
					h.updateClient(agent)
				}
			}
			h.updateClient(client)
		case agent := <-h.agentCh:
			agent.LastUpdated = time.Now().UnixMilli()
			h.agents[agent.Hostname] = agent
			h.updateAgents()
		case client := <-h.unregisterCh:
			delete(h.clients, client)
			for _, topic := range h.topics {
				delete(topic.clients, client)
			}
		case <-stopCh:
			// let all clients know
			for client := range h.clients {
				client.close()
			}
			timeout := time.NewTicker(5 * time.Second)
			for {
				select {
				case client := <-h.unregisterCh:
					delete(h.clients, client)
				case <-timeout.C:
					level.Error(h.logger).Log("msg", "timeout waiting for clients to disconnect", "clients", fmt.Sprintf("%+#v", h.clients))
					return
				}

				if len(h.clients) == 0 {
					return
				}
			}
		case f := <-h.rulesCh:
			f(h)
			t, ok := h.topics["rules"]
			if !ok {
				continue
			}
			t.get(&h.buf)
			// update all clients
			for client := range t.clients {
				h.updateClient(client)
			}
		}
	}
}

func (h *hub) updateRulesPayload(p *settingsv1.CollectionPayloadData) error {
	h.lck.RLock()
	defer h.lck.RUnlock()

	if cap(p.Rules) < len(h.rules) {
		p.Rules = make([]*settingsv1.CollectionRule, 0, len(h.rules))
	} else {
		p.Rules = p.Rules[:0]
	}
	for _, r := range h.rules {
		p.Rules = append(p.Rules, r.CloneVT())
	}
	return nil
}

func (h *hub) updateInstancesPayload(p *settingsv1.CollectionPayloadData) error {
	h.lck.RLock()
	defer h.lck.RUnlock()

	if cap(p.Instances) < len(h.agents) {
		p.Instances = make([]*settingsv1.CollectionInstance, 0, len(h.agents))
	} else {
		p.Instances = p.Instances[:0]
	}
	for _, a := range h.agents {
		p.Instances = append(p.Instances, a.CloneVT())
	}
	return nil
}

func mergePayloads(in ...*settingsv1.CollectionPayloadData) *settingsv1.CollectionPayloadData {
	var lenRules, lenInstances int
	for _, x := range in {
		lenRules += len(x.Rules)
		lenInstances += len(x.Instances)
	}

	res := &settingsv1.CollectionPayloadData{
		Rules:     make([]*settingsv1.CollectionRule, 0, lenRules),
		Instances: make([]*settingsv1.CollectionInstance, 0, lenInstances),
	}

	for _, x := range in {
		res.Rules = append(res.Rules, x.Rules...)
		res.Instances = append(res.Instances, x.Instances...)
	}
	return res
}
