package recording

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"sort"
	"strings"
	"sync"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tenant"
	prom "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/thanos-io/objstore"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/settings/v1/settingsv1connect"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/settings/store"
	"github.com/grafana/pyroscope/pkg/validation"
)

var _ settingsv1connect.RecordingRulesServiceHandler = (*RecordingRules)(nil)

func New(bucket objstore.Bucket, logger log.Logger, overrides *validation.Overrides) *RecordingRules {
	return &RecordingRules{
		bucket:    bucket,
		logger:    logger,
		stores:    make(map[store.Key]*bucketStore),
		overrides: overrides,
	}
}

// RecordingRules is a collection that gathers rules coming from config and coming from the bucket storage.
// Rules coming from config work as overrides of store rules, and in case of repeated ID, config rules prevail.
type RecordingRules struct {
	bucket objstore.Bucket
	logger log.Logger

	rw     sync.RWMutex
	stores map[store.Key]*bucketStore

	overrides *validation.Overrides
}

// GetRecordingRule will return a rule of the given ID or not found.
// Rules defined by config are returned over rules in the store.
func (r *RecordingRules) GetRecordingRule(ctx context.Context, req *connect.Request[settingsv1.GetRecordingRuleRequest]) (*connect.Response[settingsv1.GetRecordingRuleResponse], error) {
	err := validateGet(req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	tenantID, err := r.tenantOrError(ctx)
	if err != nil {
		return nil, err
	}

	// look for provisioned rules
	rulesFromConfig := r.recordingRulesFromOverrides(tenantID)
	for _, r := range rulesFromConfig {
		if r.Id == req.Msg.Id {
			return connect.NewResponse(&settingsv1.GetRecordingRuleResponse{Rule: r}), nil
		}
	}

	s := r.storeForTenant(tenantID)
	rule, err := s.Get(ctx, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if rule == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no rule with id='%s' found", req.Msg.Id))
	}

	res := &settingsv1.GetRecordingRuleResponse{
		Rule: convertRuleToAPI(rule),
	}
	return connect.NewResponse(res), nil
}

// ListRecordingRules will return all the rules defined by config and in the store. Rules in the store with the same ID
// as a rule in config will be filtered out.
func (r *RecordingRules) ListRecordingRules(ctx context.Context, req *connect.Request[settingsv1.ListRecordingRulesRequest]) (*connect.Response[settingsv1.ListRecordingRulesResponse], error) {
	tenantId, err := r.tenantOrError(ctx)
	if err != nil {
		return nil, err
	}

	rulesFromOverrides := r.recordingRulesFromOverrides(tenantId)
	ruleIds := make(map[string]struct{}, len(rulesFromOverrides))
	res := &settingsv1.ListRecordingRulesResponse{
		Rules: make([]*settingsv1.RecordingRule, 0),
	}
	for _, r := range rulesFromOverrides {
		ruleIds[r.Id] = struct{}{}
		res.Rules = append(res.Rules, r)
	}

	s := r.storeForTenant(tenantId)
	rulesFromStore, err := s.List(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	for _, rule := range rulesFromStore.Rules {
		if _, overridden := ruleIds[rule.Id]; overridden {
			continue
		}
		res.Rules = append(res.Rules, convertRuleToAPI(rule))
	}

	return connect.NewResponse(res), nil
}

// UpsertRecordingRule upserts a rule in the storage.
// Operational purposes: you can upsert store rules (no matter if they exist in config)
func (r *RecordingRules) UpsertRecordingRule(ctx context.Context, req *connect.Request[settingsv1.UpsertRecordingRuleRequest]) (*connect.Response[settingsv1.UpsertRecordingRuleResponse], error) {
	err := validateUpsert(req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid request: %v", err))
	}

	s, err := r.storeFromContext(ctx)
	if err != nil {
		return nil, err
	}

	newRule := &settingsv1.RecordingRuleStore{
		Id:               req.Msg.Id,
		MetricName:       req.Msg.MetricName,
		Matchers:         req.Msg.Matchers,
		GroupBy:          req.Msg.GroupBy,
		ExternalLabels:   req.Msg.ExternalLabels,
		Generation:       req.Msg.Generation,
		StacktraceFilter: req.Msg.StacktraceFilter,
	}
	newRule, err = s.Upsert(ctx, newRule)
	if err != nil {
		var cErr *store.ErrConflictGeneration
		if errors.As(err, &cErr) {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("conflicting update, please try again"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	res := &settingsv1.UpsertRecordingRuleResponse{
		Rule: convertRuleToAPI(newRule),
	}
	return connect.NewResponse(res), nil
}

// DeleteRecordingRule deletes a store rule
// Operational purposes: you can delete store rules (no matter if they exist in config)
func (r *RecordingRules) DeleteRecordingRule(ctx context.Context, req *connect.Request[settingsv1.DeleteRecordingRuleRequest]) (*connect.Response[settingsv1.DeleteRecordingRuleResponse], error) {
	err := validateDelete(req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid request: %v", err))
	}

	s, err := r.storeFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	err = s.Delete(ctx, req.Msg.Id)
	if err != nil {
		return nil, err
	}

	res := &settingsv1.DeleteRecordingRuleResponse{}
	return connect.NewResponse(res), nil
}

func (r *RecordingRules) storeFromContext(ctx context.Context) (*bucketStore, error) {
	tenantID, err := r.tenantOrError(ctx)
	if err != nil {
		return nil, err
	}
	return r.storeForTenant(tenantID), nil
}

func (r *RecordingRules) storeForTenant(tenantID string) *bucketStore {
	key := store.Key{TenantID: tenantID}

	r.rw.RLock()
	tenantStore, ok := r.stores[key]
	r.rw.RUnlock()
	if ok {
		return tenantStore
	}

	r.rw.Lock()
	defer r.rw.Unlock()

	tenantStore, ok = r.stores[key]
	if ok {
		return tenantStore
	}

	tenantStore = newBucketStore(r.logger, r.bucket, key)
	r.stores[key] = tenantStore
	return tenantStore
}

func (r *RecordingRules) tenantOrError(ctx context.Context) (string, error) {
	tenantID, err := tenant.TenantID(ctx)
	if err != nil {
		level.Error(r.logger).Log("error getting tenant ID", "err", err)
		return "", connect.NewError(connect.CodeInternal, err)
	}
	return tenantID, nil
}

func (r *RecordingRules) recordingRulesFromOverrides(tenantID string) []*settingsv1.RecordingRule {
	rules := r.overrides.RecordingRules(tenantID)
	for i := range rules {
		rules[i].Provisioned = true
		if rules[i].Id == "" {
			// for consistency, rules will be filled with an ID
			rules[i].Id = idForRule(rules[i])
		}
	}
	return rules
}

func validateGet(req *settingsv1.GetRecordingRuleRequest) error {
	// Format fields.
	req.Id = strings.TrimSpace(req.Id)

	// Validate fields.
	var errs []error

	if req.Id == "" {
		errs = append(errs, fmt.Errorf("id is required"))
	}

	return errors.Join(errs...)
}

var (
	upsertIdRE = regexp.MustCompile(`^[a-zA-Z]+$`)
)

func validateUpsert(req *settingsv1.UpsertRecordingRuleRequest) error {
	// Validate fields.
	var errs []error

	// Format fields.
	if req.Id == "" {
		req.Id = generateID(idLength)
		req.Generation = 1
	}
	req.MetricName = strings.TrimSpace(req.MetricName)

	if !upsertIdRE.MatchString(req.Id) {
		errs = append(errs, fmt.Errorf("id %q must match %s", req.Id, upsertIdRE.String()))
	}

	if req.MetricName == "" {
		errs = append(errs, fmt.Errorf("metric_name is required"))
	} else if err := model.ValidateMetricName(req.MetricName); err != nil {
		errs = append(errs, fmt.Errorf("metric_name %q is invalid: %v", req.MetricName, err))
	}

	for _, m := range req.Matchers {
		_, err := parser.ParseMetricSelector(m)
		if err != nil {
			errs = append(errs, fmt.Errorf("matcher %q is invalid: %v", m, err))
		}
	}

	for _, l := range req.GroupBy {
		name := prom.LabelName(l)
		if !prom.LegacyValidation.IsValidLabelName(string(name)) {
			errs = append(errs, fmt.Errorf("group_by label %q must match %s", l, prom.LabelNameRE.String()))
		}
	}

	for _, l := range req.ExternalLabels {
		name := prom.LabelName(l.Name)
		if !prom.LegacyValidation.IsValidLabelName(string(name)) {
			errs = append(errs, fmt.Errorf("external_labels name %q must match %s", name, prom.LabelNameRE.String()))
		}

		value := prom.LabelValue(l.Value)
		if !value.IsValid() {
			errs = append(errs, fmt.Errorf("external_labels value %q must be a valid utf-8 string", l.Value))
		}
	}

	if req.Generation < 0 {
		errs = append(errs, fmt.Errorf("generation must be positive"))
	}

	return errors.Join(errs...)
}

func validateDelete(req *settingsv1.DeleteRecordingRuleRequest) error {
	// Format fields.
	req.Id = strings.TrimSpace(req.Id)

	// Validate fields.
	var errs []error

	if req.Id == "" {
		errs = append(errs, fmt.Errorf("id is required"))
	}

	return errors.Join(errs...)
}

func convertRuleToAPI(rule *settingsv1.RecordingRuleStore) *settingsv1.RecordingRule {
	apiRule := &settingsv1.RecordingRule{
		Id:               rule.Id,
		MetricName:       rule.MetricName,
		ProfileType:      "unknown",
		Matchers:         rule.Matchers,
		GroupBy:          rule.GroupBy,
		ExternalLabels:   rule.ExternalLabels,
		Generation:       rule.Generation,
		StacktraceFilter: rule.StacktraceFilter,
	}

	// Try find the profile type from the matchers.
Loop:
	for _, m := range rule.Matchers {
		s, err := parser.ParseMetricSelector(m)
		if err != nil {
			// Since this value is loaded from the tenant settings database and
			// we validate selectors before saving, we should theoretically
			// always have valid selectors. If there's an error parsing a
			// selector, we'll just skip it.
			continue
		}

		for _, label := range s {
			if label.Name != model.LabelNameProfileType {
				continue
			}

			if label.Type != labels.MatchEqual {
				continue
			}

			apiRule.ProfileType = label.Value
			break Loop
		}
	}

	return apiRule
}

const (
	alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	idLength = 10
)

func generateID(length int) string {

	if length < 1 {
		return ""
	}

	b := make([]byte, length)
	for i := range b {
		b[i] = alphabet[rand.Intn(len(alphabet))]
	}
	return string(b)
}

func idForRule(rule *settingsv1.RecordingRule) string {
	var b strings.Builder
	b.WriteString(rule.MetricName)
	b.WriteString(rule.ProfileType)
	sort.Strings(rule.Matchers)
	for _, m := range rule.Matchers {
		b.WriteString(m)
	}
	sort.Strings(rule.GroupBy)
	for _, g := range rule.GroupBy {
		b.WriteString(g)
	}
	for _, l := range rule.ExternalLabels {
		b.WriteString(l.Name)
		b.WriteString(l.Value)
	}
	if rule.StacktraceFilter != nil && rule.StacktraceFilter.FunctionName != nil {
		b.WriteString(rule.StacktraceFilter.FunctionName.FunctionName)
	}
	sum := sha256.Sum256([]byte(b.String()))
	id := make([]byte, idLength)
	for i := 0; i < idLength; i++ {
		id[i] = alphabet[sum[i]%byte(len(alphabet))]
	}
	return string(id)
}
