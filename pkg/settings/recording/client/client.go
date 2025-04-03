package recording

import (
	"context"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/settings/v1/settingsv1connect"
	connectapi "github.com/grafana/pyroscope/pkg/api/connect"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/util"
)

type Client struct {
	service services.Service
	client  settingsv1connect.RecordingRulesServiceClient
	logger  log.Logger
}

func NewClient(address string, logger log.Logger, auth connect.Option) (*Client, error) {
	httpClient := util.InstrumentedDefaultHTTPClient()
	opts := connectapi.DefaultClientOptions()
	opts = append(opts, auth)
	c := Client{
		client: settingsv1connect.NewRecordingRulesServiceClient(httpClient, address, opts...),
		logger: logger,
	}
	c.service = services.NewIdleService(c.starting, c.stopping)
	return &c, nil
}

func (b *Client) RecordingRules(tenantId string) ([]*phlaremodel.RecordingRule, error) {
	ctx := tenant.InjectTenantID(context.Background(), tenantId)
	resp, err := b.client.ListRecordingRules(ctx, connect.NewRequest(&settingsv1.ListRecordingRulesRequest{}))
	if err != nil {
		return nil, err
	}
	rules := make([]*phlaremodel.RecordingRule, 0, len(resp.Msg.Rules))
	for _, rule := range resp.Msg.Rules {
		r, err := phlaremodel.NewRecordingRule(rule)
		if err == nil {
			rules = append(rules, r)
		} else {
			level.Error(b.logger).Log("msg", "failed to parse recording rule", "rule", rule, "err", err)
		}
	}
	return rules, nil
}

func (b *Client) Service() services.Service      { return b.service }
func (b *Client) starting(context.Context) error { return nil }
func (b *Client) stopping(error) error           { return nil }
