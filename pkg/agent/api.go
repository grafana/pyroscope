package agent

import (
	"context"

	"github.com/bufbuild/connect-go"
	"github.com/pkg/errors"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"

	agentv1 "github.com/grafana/fire/pkg/gen/agent/v1"
)

func (a *Agent) GetTargets(ctx context.Context, req *connect.Request[agentv1.GetTargetsRequest]) (*connect.Response[agentv1.GetTargetsResponse], error) {
	showActive := req.Msg.State == agentv1.State_STATE_UNSPECIFIED || req.Msg.State == agentv1.State_STATE_ACTIVE
	showDropped := req.Msg.State == agentv1.State_STATE_UNSPECIFIED || req.Msg.State == agentv1.State_STATE_DROPPED

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

	return connect.NewResponse(&resp), nil
}
