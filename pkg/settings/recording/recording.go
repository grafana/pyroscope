package recording

import (
	"context"
	"sync"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tenant"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/settings/v1/settingsv1connect"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/settings/store"
)

var _ settingsv1connect.RecordingRulesServiceHandler = (*RecordingRules)(nil)

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
		return nil, err
	}

	rules, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	res := &settingsv1.ListRecordingRulesResponse{
		Rules: make([]*settingsv1.RecordingRule_API, 0, len(rules.Rules)),
	}
	for _, rule := range rules.Rules {
		res.Rules = append(res.Rules, convertRuleToAPI(rule))
	}

	return connect.NewResponse(res), nil
}

func (r *RecordingRules) InsertRecordingRule(ctx context.Context, req *connect.Request[settingsv1.InsertRecordingRuleRequest]) (*connect.Response[settingsv1.InsertRecordingRuleResponse], error) {
	// TODO(bryan): Validate request

	s, err := r.storeForTenant(ctx)
	if err != nil {
		return nil, err
	}

	newRule := &settingsv1.RecordingRule_Store{
		Id:             req.Msg.Id,
		MetricName:     req.Msg.MetricName,
		Matchers:       req.Msg.Matchers,
		GroupBy:        req.Msg.GroupBy,
		ExternalLabels: req.Msg.ExternalLabels,
		DataSourceName: req.Msg.PrometheusDataSource,
	}
	newRule, err = s.insert(ctx, newRule)
	if err != nil {
		return nil, err
	}

	res := &settingsv1.InsertRecordingRuleResponse{
		Rule: convertRuleToAPI(newRule),
	}
	return connect.NewResponse(res), nil
}

func (r *RecordingRules) DeleteRecordingRule(ctx context.Context, req *connect.Request[settingsv1.DeleteRecordingRuleRequest]) (*connect.Response[settingsv1.DeleteRecordingRuleResponse], error) {
	// TODO(bryan): Validate request (id can't be blank)

	s, err := r.storeForTenant(ctx)
	if err != nil {
		return nil, err
	}

	err = s.delete(ctx, req.Msg.Id)
	if err != nil {
		return nil, err
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

func convertRuleToAPI(rule *settingsv1.RecordingRule_Store) *settingsv1.RecordingRule_API {
	apiRule := &settingsv1.RecordingRule_API{
		Id:                   rule.Id,
		MetricName:           rule.MetricName,
		ServiceName:          "unknown", // TODO(bryan) parse out service name from matchers if possible
		ProfileType:          "unknown", // TODO(bryan) parse out profile type from matchers if possible
		Matchers:             rule.Matchers,
		GroupBy:              rule.GroupBy,
		ExternalLabels:       rule.ExternalLabels,
		PrometheusDataSource: rule.DataSourceName,
	}

	return apiRule
}
