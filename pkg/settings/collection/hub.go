package collection

import (
	"context"
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
	update  func(context.Context, *settingsv1.CollectionPayloadData) error
	err     error
	content *settingsv1.CollectionPayloadData
	hash    uint64
	clients map[*client]uint64
}

func newTopic(name string, f func(context.Context, *settingsv1.CollectionPayloadData) error) *topic {
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

func (t *topic) get(ctx context.Context, b *buf) {
	t.err = t.update(ctx, t.content)
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

func (k hubKey) path() string {
	return fmt.Sprintf("%s/settings/collection.%s", k.tenantID, k.scope)
}

type hub struct {
	logger log.Logger

	store *bucketStore

	lck    sync.RWMutex // TODO: Figure out why?
	topics map[string]*topic

	clients map[*client]struct{}

	instancesPublishing       map[*client]bool // are particular instances publishing their targets
	instancesPublishingActive bool             // is instance targets publishing requested
	instances                 map[string]*settingsv1.CollectionInstance

	// Register requests from the clients.
	registerCh chan *client

	// Unregister requests from clients.
	unregisterCh chan *client

	// Update instance targets
	instanceCh chan *settingsv1.CollectionInstance

	// Update rules
	rulesCh chan func(*hub)

	// keep data and hash buffer around
	buf buf
}

func newHub(logger log.Logger, store *bucketStore, topicsF func(*hub) []*topic) *hub {
	h := &hub{
		logger:              logger,
		store:               store,
		topics:              make(map[string]*topic),
		clients:             make(map[*client]struct{}),
		instancesPublishing: make(map[*client]bool),
		instances:           make(map[string]*settingsv1.CollectionInstance),
		instanceCh:          make(chan *settingsv1.CollectionInstance),
		registerCh:          make(chan *client),
		unregisterCh:        make(chan *client),
		rulesCh:             make(chan func(*hub), 32),
	}

	for _, topic := range topicsF(h) {
		h.topics[topic.name] = topic
	}

	return h
}

// check data for topics to sent
func (h *hub) updateClientTopicsToPublish(ctx context.Context, client *client) error {
	var payload []*settingsv1.CollectionPayloadData
	for _, t := range h.topics {
		clientHash, ok := t.clients[client]
		if !ok {
			continue
		}

		if t.hash == 0 {
			t.get(ctx, &h.buf)
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

// check if instance needs toggle publishing
func (h *hub) updateClientToggleInstanceSubscription(_ context.Context, client *client) error {
	active, ok := h.instancesPublishing[client]
	if !ok {
		return nil
	}
	if active != h.instancesPublishingActive {

		msg := settingsv1.CollectionMessage{
			PayloadSubscribe: &settingsv1.CollectionPayloadSubscribe{},
		}
		if !active {
			msg.PayloadSubscribe.Topics = []string{topicInstances}
		}

		data, err := json.Marshal(&msg)
		if err != nil {
			return err
		}

		level.Debug(client.logger).Log("request instance target publishing", "client", fmt.Sprintf("%p", client), "data", string(data))
		client.send <- data

		h.instancesPublishing[client] = h.instancesPublishingActive
	}
	return nil
}

func (h *hub) updateClient(ctx context.Context, client *client) {
	if err := h.updateClientTopicsToPublish(ctx, client); err != nil {
		level.Error(client.logger).Log("msg", "error updating client topics to publish", "err", err)
	}
	if err := h.updateClientToggleInstanceSubscription(ctx, client); err != nil {
		level.Error(client.logger).Log("msg", "error updating client to subscribe to instances", "err", err)
	}
}

func (h *hub) updateInstances(ctx context.Context) {
	t, ok := h.topics[topicInstances]
	if !ok {
		return
	}
	t.get(ctx, &h.buf)

	// update frontend with new instance data
	for client := range t.clients {
		h.updateClient(ctx, client)
	}
}

func (a *hub) insertRule(ctx context.Context, data *settingsv1.CollectionPayloadRuleInsert) error {
	rc, err := CollectionRuleToRelabelConfig(data.Rule)
	if err != nil {
		return err
	}
	if err := rc.Validate(); err != nil {
		return err
	}

	return a.store.insertRule(ctx, data)
}

func (h *hub) deleteRule(ctx context.Context, id int64) error {
	return h.store.deleteRule(ctx, id)
}

func (h *hub) run(stopCh <-chan struct{}) {
	ctx := context.Background()
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
				h.instancesPublishing[client] = false
			}

			// check if instance need toggle publishing
			instanceTopic, ok := h.topics[topicInstances]
			instancesPublishingRequested := false
			if ok {
				instancesPublishingRequested = len(instanceTopic.clients) != 0
			}
			if instancesPublishingRequested != h.instancesPublishingActive {
				if instancesPublishingRequested {
					slog.Debug("instance targets publishing has been enabled")
				} else {
					slog.Debug("instance targets publishing has been disabled")
				}
				h.instancesPublishingActive = instancesPublishingRequested
				// send message to all instances
				for instance := range h.instancesPublishing {
					h.updateClient(ctx, instance)
				}
			}
			h.updateClient(ctx, client)
		case instance := <-h.instanceCh:
			instance.LastUpdated = time.Now().UnixMilli()
			h.instances[instance.Hostname] = instance
			h.updateInstances(ctx)
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
			t, ok := h.topics[topicRules]
			if !ok {
				continue
			}
			t.get(ctx, &h.buf)
			// update all clients
			for client := range t.clients {
				h.updateClient(ctx, client)
			}
		}
	}
}

func (h *hub) updateRulesPayload(ctx context.Context, p *settingsv1.CollectionPayloadData) error {
	rules, err := h.store.list(ctx)
	if err != nil {
		return fmt.Errorf("error reading rules from store: %w", err)
	}
	if cap(p.Rules) < len(rules) {
		p.Rules = make([]*settingsv1.CollectionRule, 0, len(rules))
	} else {
		p.Rules = p.Rules[:0]
	}
	for _, r := range rules {
		p.Rules = append(p.Rules, r.CloneVT())
	}
	return nil
}

func (h *hub) updateInstancesPayload(ctx context.Context, p *settingsv1.CollectionPayloadData) error {
	h.lck.RLock()
	defer h.lck.RUnlock()

	if cap(p.Instances) < len(h.instances) {
		p.Instances = make([]*settingsv1.CollectionInstance, 0, len(h.instances))
	} else {
		p.Instances = p.Instances[:0]
	}
	for _, a := range h.instances {
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
