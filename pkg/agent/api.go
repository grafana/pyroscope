package agent

import (
	"context"

	"github.com/bufbuild/connect-go"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	agentv1 "github.com/grafana/phlare/api/gen/proto/go/agent/v1"
	"github.com/grafana/phlare/api/gen/proto/go/agent/v1/agentv1connect"
)

func (a *Agent) GetTargets(ctx context.Context, req *agentv1.GetTargetsRequest) (*agentv1.GetTargetsResponse, error) {
	showActive := req.State == agentv1.State_STATE_UNSPECIFIED || req.State == agentv1.State_STATE_ACTIVE
	showDropped := req.State == agentv1.State_STATE_UNSPECIFIED || req.State == agentv1.State_STATE_DROPPED

	resp := agentv1.GetTargetsResponse{}

	if showActive {
		targetsActive := a.ActiveTargets()
		resp.ActiveTargets = make([]*agentv1.Target, 0, len(targetsActive))
		for group, tg := range targetsActive {
			for _, t := range tg {
				lastErrStr := ""
				lastErr := t.LastError()
				if lastErr != nil {
					lastErrStr = lastErr.Error()
				}

				var err error
				resp.ActiveTargets = append(resp.ActiveTargets, &agentv1.Target{
					Labels:           t.Labels().Map(),
					DiscoveredLabels: t.Target.DiscoveredLabels().Map(),
					ScrapePool:       group,
					ScrapeUrl:        t.URL().String(),
					LastError: func() string {
						if err == nil && lastErrStr == "" {
							return ""
						} else if err != nil {
							return errors.Wrapf(err, lastErrStr).Error()
						}
						return lastErrStr
					}(),
					LastScrape:         timestamppb.New(t.LastScrape()),
					LastScrapeDuration: durationpb.New(t.LastScrapeDuration()),
					Health:             t.Health(),
					ScrapeInterval:     durationpb.New(t.interval),
					ScrapeTimeout:      durationpb.New(t.timeout),
				})
			}
		}
	}

	if showDropped {
		tDropped := a.DroppedTargets()
		resp.DroppedTargets = make([]*agentv1.Target, 0, len(tDropped))
		for _, t := range tDropped {
			resp.DroppedTargets = append(resp.DroppedTargets, &agentv1.Target{
				Labels:           t.Labels().Map(),
				DiscoveredLabels: t.Target.DiscoveredLabels().Map(),
				ScrapeUrl:        t.URL().String(),
			})
		}
	}

	return &resp, nil
}

type connectAgent struct {
	*Agent
}

func (ca *connectAgent) GetTargets(ctx context.Context, req *connect.Request[agentv1.GetTargetsRequest]) (*connect.Response[agentv1.GetTargetsResponse], error) {
	resp, err := ca.Agent.GetTargets(ctx, req.Msg)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(resp), nil
}

func (a *Agent) ConnectHandler() agentv1connect.AgentServiceHandler {
	return &connectAgent{a}
}
