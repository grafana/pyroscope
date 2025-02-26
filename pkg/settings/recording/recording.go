package recording

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"regexp"
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
)

var _ settingsv1connect.RecordingRulesServiceHandler = (*RecordingRules)(nil)

func New(cfg Config, bucket objstore.Bucket, logger log.Logger) *RecordingRules {
	return &RecordingRules{
		cfg:    cfg,
		bucket: bucket,
		logger: logger,
		stores: make(map[store.Key]*bucketStore),
	}
}

type RecordingRules struct {
	cfg    Config
	bucket objstore.Bucket
	logger log.Logger

	rw     sync.RWMutex
	stores map[store.Key]*bucketStore
}

func (r *RecordingRules) GetRecordingRule(ctx context.Context, req *connect.Request[settingsv1.GetRecordingRuleRequest]) (*connect.Response[settingsv1.GetRecordingRuleResponse], error) {
	err := validateGet(req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	s, err := r.storeForTenant(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	rule, err := s.Get(ctx, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	res := &settingsv1.GetRecordingRuleResponse{
		Rule: convertRuleToAPI(rule),
	}
	return connect.NewResponse(res), nil
}

func (r *RecordingRules) ListRecordingRules(ctx context.Context, req *connect.Request[settingsv1.ListRecordingRulesRequest]) (*connect.Response[settingsv1.ListRecordingRulesResponse], error) {
	s, err := r.storeForTenant(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	rules, err := s.List(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	res := &settingsv1.ListRecordingRulesResponse{
		Rules: make([]*settingsv1.RecordingRule, 0, len(rules.Rules)),
	}
	for _, rule := range rules.Rules {
		res.Rules = append(res.Rules, convertRuleToAPI(rule))
	}

	return connect.NewResponse(res), nil
}

func (r *RecordingRules) UpsertRecordingRule(ctx context.Context, req *connect.Request[settingsv1.UpsertRecordingRuleRequest]) (*connect.Response[settingsv1.UpsertRecordingRuleResponse], error) {
	err := validateUpsert(req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid request: %v", err))
	}

	s, err := r.storeForTenant(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	newRule := &settingsv1.RecordingRuleStore{
		Id:             req.Msg.Id,
		MetricName:     req.Msg.MetricName,
		Matchers:       req.Msg.Matchers,
		GroupBy:        req.Msg.GroupBy,
		ExternalLabels: req.Msg.ExternalLabels,
		Generation:     req.Msg.Generation,
	}
	newRule, err = s.Upsert(ctx, newRule)
	if err != nil {
		var cErr *store.ErrConflictGeneration
		if errors.As(err, &cErr) {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("Conflicting update, please try again"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	res := &settingsv1.UpsertRecordingRuleResponse{
		Rule: convertRuleToAPI(newRule),
	}
	return connect.NewResponse(res), nil
}

func (r *RecordingRules) DeleteRecordingRule(ctx context.Context, req *connect.Request[settingsv1.DeleteRecordingRuleRequest]) (*connect.Response[settingsv1.DeleteRecordingRuleResponse], error) {
	err := validateDelete(req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid request: %v", err))
	}

	s, err := r.storeForTenant(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	err = s.Delete(ctx, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	res := &settingsv1.DeleteRecordingRuleResponse{}
	return connect.NewResponse(res), nil
}

func (r *RecordingRules) storeForTenant(ctx context.Context) (*bucketStore, error) {
	tenantID, err := tenant.TenantID(ctx)
	if err != nil {
		level.Error(r.logger).Log("error getting tenant ID", "err", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	key := store.Key{TenantID: tenantID}

	r.rw.RLock()
	tenantStore, ok := r.stores[key]
	r.rw.RUnlock()
	if ok {
		return tenantStore, nil
	}

	r.rw.Lock()
	defer r.rw.Unlock()

	tenantStore, ok = r.stores[key]
	if ok {
		return tenantStore, nil
	}

	tenantStore = newBucketStore(r.logger, r.bucket, key)
	r.stores[key] = tenantStore
	return tenantStore, nil
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
	// Format fields.
	if req.Id == "" {
		req.Id = generateID(10)
		req.Generation = 1
	}
	req.MetricName = strings.TrimSpace(req.MetricName)

	// Validate fields.
	var errs []error

	if !upsertIdRE.MatchString(req.Id) {
		errs = append(errs, fmt.Errorf("id %q must match %s", req.Id, upsertIdRE.String()))
	}

	if req.MetricName == "" {
		errs = append(errs, fmt.Errorf("metric_name is required"))
	} else if !prom.IsValidMetricName(prom.LabelValue(req.MetricName)) {
		errs = append(errs, fmt.Errorf("metric_name %q must be a valid utf-8 string", req.MetricName))
	}

	for _, m := range req.Matchers {
		_, err := parser.ParseMetricSelector(m)
		if err != nil {
			errs = append(errs, fmt.Errorf("matcher %q is invalid: %v", m, err))
		}
	}

	for _, l := range req.GroupBy {
		name := prom.LabelName(l)
		if !name.IsValid() {
			errs = append(errs, fmt.Errorf("group_by label %q must match %s", l, prom.LabelNameRE.String()))
		}
	}

	for _, l := range req.ExternalLabels {
		name := prom.LabelName(l.Name)
		if !name.IsValid() {
			errs = append(errs, fmt.Errorf("external_labels name %q must be a valid utf-8 string", l.Name))
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
		Id:             rule.Id,
		MetricName:     rule.MetricName,
		ProfileType:    "unknown",
		Matchers:       rule.Matchers,
		GroupBy:        rule.GroupBy,
		ExternalLabels: rule.ExternalLabels,
		Generation:     rule.Generation,
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

func generateID(length int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

	if length < 1 {
		return ""
	}

	b := make([]byte, length)
	for i := range b {
		b[i] = alphabet[rand.Intn(len(alphabet))]
	}
	return string(b)
}
