package main

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-kit/log/level"
	"github.com/gorilla/websocket"
	jsoniter "github.com/json-iterator/go"
	"github.com/prometheus/prometheus/model/relabel"
	"golang.org/x/exp/rand"
	"gopkg.in/yaml.v3"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/pkg/settings/collection"
)

type collectionRulesParams struct {
	*phlareClient
	Scope string
}

func addCollectionRulesParams(collectionRuleCmd commander) *collectionRulesParams {
	params := new(collectionRulesParams)
	params.phlareClient = addPhlareClient(collectionRuleCmd)
	collectionRuleCmd.Flag("scope", "Collection rule scope.").Default("alloy").StringVar(&params.Scope)
	return params
}

type noopRoundTripper struct{}

func (n *noopRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return new(http.Response), nil
}

type rulesClient struct {
	conn *websocket.Conn
	id   int64
}

func (c *rulesClient) msgID() int64 {
	if c.id == 0 {
		randomId := rand.Int63()
		c.id = randomId
		return randomId
	}
	c.id++
	return c.id
}

func msgType(msg *settingsv1.CollectionMessage) string {
	if msg.PayloadRuleInsert != nil {
		return "settings.v1.CollectionPayloadRuleInsert"
	}
	if msg.PayloadSubscribe != nil {
		return "settings.v1.CollectionPayloadSubscribe"
	}
	if msg.PayloadData != nil {
		return "settings.v1.CollectionPayloadData"
	}
	if msg.PayloadRuleDelete != nil {
		return "settings.v1.CollectionPayloadRuleDelete"
	}
	return ""
}

func (c *rulesClient) request(ctx context.Context, req *settingsv1.CollectionMessage) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	if req.Id == 0 {
		req.Id = c.msgID()
	}
	level.Debug(logger).Log("msg", "request", "msg.id", req.Id, "msg.type", msgType(req))

	if err := c.conn.WriteJSON(req); err != nil {
		return err
	}

	errChan := make(chan error)
	resp := new(settingsv1.CollectionMessage)

	go func() {
		for {
			resp.Reset()
			if err := c.conn.ReadJSON(resp); err != nil {
				errChan <- err
				return
			}
			if req.Id != resp.Id {
				continue
			}
			errChan <- nil
			return
		}
	}()

	select {
	case err := <-errChan:
		if err != nil {
			return err
		}
		level.Debug(logger).Log("msg", "received resp", "msg.id", resp.Id, "msg.type", msgType(req))

		if resp.Status != settingsv1.Status_STATUS_OK {
			if resp.Message != nil {
				return fmt.Errorf("status: %s message: %s", resp.Status.String(), *resp.Message)
			}
			return fmt.Errorf("status: %s", resp.Status.String())
		}

	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

func (c *rulesClient) Insert(ctx context.Context, rule *settingsv1.CollectionRule, after *int64) error {
	return c.request(ctx, &settingsv1.CollectionMessage{
		Id: c.msgID(),
		PayloadRuleInsert: &settingsv1.CollectionPayloadRuleInsert{
			After: after,
			Rule:  rule,
		},
	})
}

func (c *rulesClient) Delete(ctx context.Context, id int64) error {
	return c.request(ctx, &settingsv1.CollectionMessage{
		Id: c.msgID(),
		PayloadRuleDelete: &settingsv1.CollectionPayloadRuleDelete{
			Id: id,
		},
	})
}

func (c *rulesClient) List(ctx context.Context) ([]*settingsv1.CollectionRule, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	req := &settingsv1.CollectionMessage{
		Id: c.msgID(),
		PayloadSubscribe: &settingsv1.CollectionPayloadSubscribe{
			Topics: []string{"rules"},
		},
	}
	level.Debug(logger).Log("msg", "subscribed to rules", "id", req.Id)

	if err := c.conn.WriteJSON(req); err != nil {
		return nil, err
	}

	errChan := make(chan error)
	resp := new(settingsv1.CollectionMessage)

	go func() {
		for {
			resp.Reset()
			if err := c.conn.ReadJSON(resp); err != nil {
				errChan <- err
				return
			}
			if resp.PayloadData == nil {
				continue
			}
			errChan <- nil
			return
		}
	}()

	select {
	case err := <-errChan:
		if err != nil {
			return nil, err
		}

		level.Debug(logger).Log("msg", "received rules", "rules", len(resp.PayloadData.Rules))
		return resp.PayloadData.Rules, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *phlareClient) collectionRulesConn(endpoint string, scope string) (*rulesClient, error) {
	req, err := http.NewRequest("GET", p.URL, nil)
	if err != nil {
		return nil, err
	}

	// replace http with ws and https with wss
	if strings.HasPrefix(req.URL.Scheme, "http") {
		req.URL.Scheme = "ws" + req.URL.Scheme[4:]
	}

	req.URL.Path = filepath.Join(
		req.URL.Path,
		endpoint,
	)
	query := req.URL.Query()
	query.Add("scope", scope)
	req.URL.RawQuery = query.Encode()

	// go through roundtripper to add auth headers
	r := &authRoundTripper{
		client: p,
		next:   &noopRoundTripper{},
	}
	_, err = r.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	level.Debug(logger).Log("msg", "opening websocket", "url", req.URL.String())
	c, resp, err := websocket.DefaultDialer.Dial(req.URL.String(), req.Header)
	if err != nil {
		return nil, fmt.Errorf("failed to open websocket status_code=%d message=%s: %w", resp.StatusCode, resp.Status, err)
	}
	return &rulesClient{conn: c}, nil
}

func (p *phlareClient) collectionRulesUpdater(scope string) (*rulesClient, error) {
	return p.collectionRulesConn("settings.v1.SettingsService/UpdateCollectionRules", scope)
}

func (p *phlareClient) CollectionRulesGetter(scope string) (*rulesClient, error) {
	return p.collectionRulesConn("settings.v1.SettingsService/GetCollectionRules", scope)
}

func collectionRulesList(ctx context.Context, params *collectionRulesParams) error {
	c, err := params.collectionRulesUpdater(params.Scope)
	if err != nil {
		return err
	}

	rules, err := c.List(ctx)
	if err != nil {
		return err
	}

	bytes, err := jsoniter.Marshal(rules)
	if err != nil {
		return err
	}

	var yamlIntf []interface{}
	err = yaml.Unmarshal(bytes, &yamlIntf)
	if err != nil {
		return err
	}

	yamlBytes, err := yaml.Marshal(yamlIntf)
	if err != nil {
		return err
	}

	fmt.Fprintln(output(ctx), string(yamlBytes))

	return nil
}

type collectionRulesInsertParams struct {
	*collectionRulesParams
	afterRuleID  *int64
	sourceLabels []string
	separator    *string
	regex        *string
	modulus      *uint64
	targetLabel  *string
	replacement  *string
	action       string
}

func addCollectionRulesInsertParams(collectionRuleCmd commander) *collectionRulesInsertParams {
	params := new(collectionRulesInsertParams)
	params.collectionRulesParams = addCollectionRulesParams(collectionRuleCmd)
	collectionRuleCmd.Flag("after-rule-id", "The rule id after which this rule should be inserted.").Int64Var(params.afterRuleID)
	collectionRuleCmd.Flag("source-label", "A list of labels from which values are taken and concatenatedwith the configured separator in order.").StringsVar(&params.sourceLabels)
	collectionRuleCmd.Flag("separator", "Separator is the string between concatenated values from the source labels.").StringVar(params.separator)
	collectionRuleCmd.Flag("regex", "Regex against which the concatenation is matched.").StringVar(params.regex)
	collectionRuleCmd.Flag("modulus", "Modulus to take of the hash of concatenated values from the source labels.").Uint64Var(params.modulus)
	collectionRuleCmd.Flag("target-label", "TargetLabel is the label to which the resulting string is written in a replacement. Regexp interpolation is allowed for the replace action.").StringVar(params.targetLabel)
	collectionRuleCmd.Flag("replacement", "Replacement is the regex replacement pattern to be used.").StringVar(params.replacement)
	collectionRuleCmd.Flag("action", "Action is the action to be performed for the relabeling.").Required().EnumVar(
		&params.action,
		string(relabel.Replace),
		string(relabel.Keep),
		string(relabel.Drop),
		string(relabel.KeepEqual),
		string(relabel.DropEqual),
		string(relabel.HashMod),
		string(relabel.LabelMap),
		string(relabel.LabelDrop),
		string(relabel.LabelKeep),
		string(relabel.Lowercase),
		string(relabel.Uppercase),
	)
	return params
}

func ruleStringToCollectionRule(rule string) (settingsv1.CollectionRuleAction, error) {
	key := "COLLECTION_RULE_ACTION_" + strings.ToUpper(rule)

	x, ok := settingsv1.CollectionRuleAction_value[key]
	if !ok {
		return settingsv1.CollectionRuleAction_COLLECTION_RULE_ACTION_UNSPECIFIED, fmt.Errorf("invalid action: %s", rule)
	}

	return settingsv1.CollectionRuleAction(x), nil
}

func collectionRulesInsert(ctx context.Context, params *collectionRulesInsertParams) error {
	action, err := ruleStringToCollectionRule(params.action)
	if err != nil {
		return err
	}

	rule := &settingsv1.CollectionRule{
		SourceLabels: params.sourceLabels,
		Separator:    params.separator,
		Regex:        params.regex,
		Modulus:      params.modulus,
		TargetLabel:  params.targetLabel,
		Replacement:  params.replacement,
		Action:       action,
	}

	x, err := collection.CollectionRuleToRelabelConfig(rule)
	if err != nil {
		return err
	}

	err = x.Validate()
	if err != nil {
		return err
	}

	c, err := params.collectionRulesUpdater(params.Scope)
	if err != nil {
		return err
	}

	return c.Insert(ctx, rule, params.afterRuleID)
}

type collectionRulesDeleteParams struct {
	*collectionRulesParams
	ruleID int64
}

func addCollectionRulesDeleteParams(collectionRuleCmd commander) *collectionRulesDeleteParams {
	params := new(collectionRulesDeleteParams)
	params.collectionRulesParams = addCollectionRulesParams(collectionRuleCmd)
	collectionRuleCmd.Flag("rule-id", "The rule id to be deleted.").Required().Int64Var(&params.ruleID)
	return params
}

func collectionRulesDelete(ctx context.Context, params *collectionRulesDeleteParams) error {
	c, err := params.collectionRulesUpdater(params.Scope)
	if err != nil {
		return err
	}

	return c.Delete(ctx, params.ruleID)
}
