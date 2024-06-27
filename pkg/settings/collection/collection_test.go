package collection

import (
	"encoding/json"
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
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"gopkg.in/yaml.v3"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
)

var (
	payloadSubscribeRules = []byte(`{"payload_subscribe":{"topics":["rules"]}}`)
)

func ruleMsg(rule *settingsv1.CollectionRule, after int64) *settingsv1.CollectionMessage {
	var afterPtr *int64
	if after != 0 {
		afterPtr = &after
	}
	return &settingsv1.CollectionMessage{
		PayloadRuleInsert: &settingsv1.CollectionPayloadRuleInsert{
			Rule:  rule,
			After: afterPtr,
		},
	}
}

func stringPtr(s string) *string {
	return &s
}

func rulesToJson(t *testing.T, rules []*settingsv1.CollectionRule) string {
	rc, err := CollectionRulesToRelabelConfigs(rules)
	require.NoError(t, err)

	// first we need to convert to yaml, to ensure the marshalling is correct
	bYAML, err := yaml.Marshal(&rc)
	require.NoError(t, err)

	var m []interface{}
	err = yaml.Unmarshal(bYAML, &m)
	require.NoError(t, err)

	// now finally time for json
	bJSON, err := json.Marshal(m)
	require.NoError(t, err)
	return string(bJSON)
}

func TestCollection_TwoCollectionInstances_OneRuleManager(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Setup code here
	var logger = log.NewNopLogger()
	if testing.Verbose() {
		logger = log.NewLogfmtLogger(os.Stderr)
	}
	c := New(Config{}, logger)
	defer c.Stop()

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
	// drop everything else
	require.NoError(t, ruleManager.WriteJSON(ruleMsg(
		&settingsv1.CollectionRule{
			Action: settingsv1.CollectionRuleAction_COLLECTION_RULE_ACTION_DROP,
		}, 0)))

	// keep loki service
	require.NoError(t, ruleManager.WriteJSON(ruleMsg(
		&settingsv1.CollectionRule{
			Action:       settingsv1.CollectionRuleAction_COLLECTION_RULE_ACTION_KEEP,
			SourceLabels: []string{"service_name"},
			Regex:        stringPtr("loki-.*"),
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
	}, 500000*time.Millisecond, time.Millisecond)

	// setup clients
	collection1, _, err := websocket.DefaultDialer.Dial(u+"/collection", nil)
	require.NoError(t, err)
	collection2, _, err := websocket.DefaultDialer.Dial(u+"/collection", nil)
	require.NoError(t, err)

	require.NoError(t, collection1.WriteMessage(websocket.TextMessage, payloadSubscribeRules))
	require.NoError(t, collection2.WriteMessage(websocket.TextMessage, payloadSubscribeRules))

	var wg sync.WaitGroup
	receiveRules := func(c *websocket.Conn) {
		defer wg.Done()
		var msg settingsv1.CollectionMessage
		for {
			err := c.ReadJSON(&msg)
			require.NoError(t, err)

			if p := msg.PayloadData; p != nil {
				// validate rules
				require.JSONEq(t, `[
          {"action":"keep","regex":"loki-.*","replacement":"$1","separator":";","source_labels":["service_name"]},
          {"action":"drop","regex":"(.*)","replacement":"$1","separator":";"}
        ]`, rulesToJson(t, p.Rules))

				break
			}
		}
	}
	wg.Add(2)
	receiveRules(collection1)
	receiveRules(collection2)

	wg.Wait()

}
