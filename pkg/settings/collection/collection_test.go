package collection

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/gorilla/websocket"
	"github.com/grafana/dskit/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
)

var (
	payloadSubscribeRules = []byte(`{"Payload":{"PayloadSubscribe":{"topics":["rules"]}}}`)
)

func ruleMsg(rule settingsv1.CollectionRule, after int64) *settingsv1.CollectionMessage {
	var afterPtr *int64
	if after != 0 {
		afterPtr = &after
	}
	return &settingsv1.CollectionMessage{
		Payload: &settingsv1.CollectionMessage_PayloadRuleInsert{
			PayloadRuleInsert: &settingsv1.CollectionPayloadRuleInsert{
				Rule:  &rule,
				After: afterPtr,
			},
		},
	}
}

func stringPtr(s string) *string {
	return &s
}

func TestCollection_TwoCollectionInstances_OneRuleManager(t *testing.T) {
	// Setup code here
	var logger = log.NewNopLogger()
	if testing.Verbose() {
		logger = log.NewLogfmtLogger(os.Stderr)
	}
	c := New(Config{}, logger)

	// Create test server with the echo handler.
	mux := http.NewServeMux()
	mux.HandleFunc("/collection", func(w http.ResponseWriter, r *http.Request) {
		c.handleWS(w, r, RuleReceiver)
	})
	mux.HandleFunc("/manager", func(w http.ResponseWriter, r *http.Request) {
		c.handleWS(w, r, RuleManager)
	})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(user.InjectOrgID(r.Context(), "my-tenant"))
		mux.ServeHTTP(w, r)
	}))
	defer s.Close()

	u := "ws" + strings.TrimPrefix(s.URL, "http")

	// setup rules
	ruleManager, _, err := websocket.DefaultDialer.Dial(u+"/manager", nil)
	require.NoError(t, err)

	// keep loki service
	require.NoError(t, ruleManager.WriteJSON(ruleMsg(
		settingsv1.CollectionRule{
			Action:       settingsv1.CollectionRuleAction_COLLECTION_RULE_ACTION_KEEP,
			SourceLabels: []string{"service_name"},
			Regex:        stringPtr("loki-.*"),
		}, 0)))

	// drop everything else
	require.NoError(t, ruleManager.WriteJSON(ruleMsg(
		settingsv1.CollectionRule{
			Action: settingsv1.CollectionRuleAction_COLLECTION_RULE_ACTION_DROP,
		}, 0)))

	// validate rules have been updated
	require.Eventually(t, func() bool {
		c.lck.RLock()
		h, ok := c.hubs[hubKey{tenantID: "my-tenant", scope: defaultScope}]
		if !ok {
			c.lck.RUnlock()
			return false
		}
		c.lck.RUnlock()

		h.lck.RLock()
		defer h.lck.RUnlock()

		return len(h.rules) == 2
	}, 50*time.Millisecond, time.Millisecond)

	// setup clients
	collection1, _, err := websocket.DefaultDialer.Dial(u+"/collection", nil)
	require.NoError(t, err)
	collection2, _, err := websocket.DefaultDialer.Dial(u+"/collection", nil)
	require.NoError(t, err)

	// TODO: Add some rules to the collection

	require.NoError(t, collection1.WriteMessage(websocket.TextMessage, payloadSubscribeRules))
	require.NoError(t, collection2.WriteMessage(websocket.TextMessage, payloadSubscribeRules))

	var wg sync.WaitGroup
	receiveRules := func(c *websocket.Conn) {
		defer wg.Done()
		var msg settingsv1.CollectionMessage
		for {
			err := collection2.ReadJSON(&msg)
			require.NoError(t, err)

			if p, ok := msg.GetPayload().(*settingsv1.CollectionMessage_PayloadData); ok {
				// validate rules
				assert.Equal(t, 2, len(p.PayloadData.Rules), "expect two rules")
				break
			}
		}
	}
	wg.Add(2)
	receiveRules(collection1)
	receiveRules(collection2)

	wg.Wait()

}
