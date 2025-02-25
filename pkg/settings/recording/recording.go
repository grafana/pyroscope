package recording

import (
	"context"
	"errors"
	"fmt"
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
	"golang.org/x/exp/rand"

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

func (r *RecordingRules) InsertRecordingRule(ctx context.Context, req *connect.Request[settingsv1.InsertRecordingRuleRequest]) (*connect.Response[settingsv1.InsertRecordingRuleResponse], error) {
	err := validateInsert(req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid request: %v", err))
	}

	s, err := r.storeForTenant(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	newRule := &settingsv1.RecordingRuleStore{
		Id:                   generateID(10),
		MetricName:           req.Msg.MetricName,
		Matchers:             req.Msg.Matchers,
		GroupBy:              req.Msg.GroupBy,
		ExternalLabels:       req.Msg.ExternalLabels,
		PrometheusDataSource: req.Msg.PrometheusDataSource,
	}
	newRule, err = s.Insert(ctx, newRule)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	res := &settingsv1.InsertRecordingRuleResponse{
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

func validateInsert(req *settingsv1.InsertRecordingRuleRequest) error {
	// Format fields.
	req.MetricName = strings.TrimSpace(req.MetricName)
	req.PrometheusDataSource = strings.TrimSpace(req.PrometheusDataSource)

	// Validate fields.
	var errs []error

	if req.MetricName == "" {
		errs = append(errs, fmt.Errorf("metric_name is required"))
	} else if !prom.IsValidMetricName(prom.LabelValue(req.MetricName)) {
		errs = append(errs, fmt.Errorf("metric_name %q must match %s", req.MetricName, prom.MetricNameRE.String()))
	}

	if req.PrometheusDataSource == "" {
		errs = append(errs, fmt.Errorf("prometheus_data_source is required"))
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
		Id:                   rule.Id,
		MetricName:           rule.MetricName,
		ProfileType:          "unknown",
		Matchers:             rule.Matchers,
		GroupBy:              rule.GroupBy,
		ExternalLabels:       rule.ExternalLabels,
		PrometheusDataSource: rule.PrometheusDataSource,
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
