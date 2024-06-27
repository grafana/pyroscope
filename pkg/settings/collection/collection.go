package collection

import (
	"fmt"
	"net/http"
	"regexp"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gorilla/websocket"
	"github.com/grafana/dskit/tenant"
)

type Config struct{}

const (
	// topic for rule updates
	topicRules = "rules"
	// topic for instance updates
	topicInstances = "instances"
)

// Collection handles the communication with Grafana Alloy, and ensures that subscribed instance received updates to rules.
// For each tenant and scope a new hub is created.
type Collection struct {
	cfg    Config
	logger log.Logger
	wg     sync.WaitGroup
	stopCh chan struct{}

	lck sync.RWMutex

	upgrader websocket.Upgrader
	hubs     map[hubKey]*hub
}

func New(cfg Config, logger log.Logger) *Collection {
	return &Collection{
		cfg:    cfg,
		logger: logger,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(r *http.Request) bool { return true }, // TODO: check origin
		},
		hubs:   make(map[hubKey]*hub),
		stopCh: make(chan struct{}),
	}
}

type Role int

const (
	RuleReceiver Role = 1 << iota
	RuleManager
)

var (
	validScopeName      = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	errInvalidScopeName = fmt.Errorf("invalid scope name, must match %s", validScopeName)
)

func (c *Collection) Stop() {
	close(c.stopCh)
	c.wg.Wait()
}

// serveWs handles websocket requests from the peer.
func (c *Collection) handleWS(w http.ResponseWriter, r *http.Request, role Role) {
	tenantID, err := tenant.TenantID(r.Context())
	if err != nil {
		level.Warn(c.logger).Log("error getting tenant ID", "err", err)
		return
	}

	// get request parameter scope from r
	scope := defaultScope
	if r.URL.Query().Has("scope") {
		paramScope := r.URL.Query().Get("scope")
		if validScopeName.MatchString(paramScope) {
			scope = paramScope
		} else {
			level.Warn(c.logger).Log("err", errInvalidScopeName, "scope", paramScope)
			http.Error(w, errInvalidScopeName.Error(), http.StatusBadRequest)
			return
		}
	}

	conn, err := c.upgrader.Upgrade(w, r, nil)
	if err != nil {
		level.Warn(c.logger).Log("error upgrading websocket", "err", err)
		return
	}

	hub := c.getHub(hubKey{tenantID: tenantID, scope: scope})

	client := &client{
		hub:  hub,
		conn: conn,
		send: make(chan []byte, 256),
		role: role,
	}
	client.logger = log.With(hub.logger, "remote", r.RemoteAddr, "user-agent", r.Header.Get("user-agent"), "client", fmt.Sprintf("%p", client))
	level.Debug(client.logger).Log("msg", "new websocket client", "is-rule-manager", client.isRuleManager())
	hub.registerCh <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	c.wg.Add(2)
	go func() {
		defer c.wg.Done()
		client.writePump()
	}()
	go func() {
		defer c.wg.Done()
		client.readPump()
	}()
}

func defaultTopics(h *hub) []*topic {
	return []*topic{
		newTopic(topicInstances, h.updateInstancesPayload),
		newTopic(topicRules, h.updateRulesPayload),
	}
}

func (c *Collection) getHub(k hubKey) *hub {
	c.lck.RLock()
	h, ok := c.hubs[k]
	if ok {
		c.lck.RUnlock()
		return h
	}
	c.lck.RUnlock()

	// now get write lock and recheck
	c.lck.Lock()
	defer c.lck.Unlock()
	h, ok = c.hubs[k]
	if ok {
		return h
	}

	h = newHub(
		log.With(c.logger, "tenant", k.tenantID, "scope", k.scope),
		defaultTopics,
	)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		h.run(c.stopCh)
	}()
	c.hubs[k] = h
	return h
}
