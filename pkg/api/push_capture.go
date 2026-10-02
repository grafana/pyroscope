package api

import (
	"context"

	"connectrpc.com/connect"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/profiledump"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

// registerPusher wraps only the external service. Legacy ingestion retains its
// direct distributor reference, so converted profiles cannot enter this hook.
func (a *API) registerPusher(svc pushv1connect.PusherServiceHandler, limits *validation.Overrides, recorder *profiledump.Recorder) {
	pushv1connect.RegisterPusherServiceHandler(a.server.HTTP, &capturePusher{next: svc, recorder: recorder}, a.connectOptionsAuthDelayRecovery(limits)...)
}

type capturePusher struct {
	next     pushv1connect.PusherServiceHandler
	recorder *profiledump.Recorder
}

func (p *capturePusher) Push(ctx context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	if tenantID, err := tenant.ExtractTenantIDFromContext(ctx); err == nil && p.recorder.PolicyActive(tenantID) {
		for _, series := range req.Msg.Series {
			if series == nil || len(series.Samples) == 0 {
				continue
			}
			// Selector lookup borrows all labels, including values omitted from metadata.
			capture := p.recorder.PrepareSeries(tenantID, model.Labels(series.Labels))
			metadataLabels := captureLabels(series.Labels)
			for _, sample := range series.Samples {
				if sample == nil {
					continue
				}
				profileID := sample.ID
				if profiledump.ValidateOriginalProfileID(profileID) != nil {
					profileID = ""
				}
				encoding := "identity"
				// The gzip signature describes encoding without validating the payload.
				if len(sample.RawProfile) >= 2 && sample.RawProfile[0] == 0x1f && sample.RawProfile[1] == 0x8b {
					encoding = "gzip"
				}
				capture.Capture(ctx, profiledump.Candidate{
					Metadata: profiledump.Metadata{
						SourceProtocol:    profiledump.SourceConnect,
						NativeFormat:      profiledump.FormatPprof,
						PayloadEncoding:   encoding,
						Labels:            metadataLabels,
						OriginalProfileID: profileID,
					},
					Payload: sample.RawProfile,
				})
			}
		}
	}
	// Capture synchronously owns accepted bytes before downstream can reuse them.
	return p.next.Push(ctx, req)
}

// Keep a bounded, representable subset in wire order without changing values.
// Selector matching above uses all external labels, including omitted metadata.
func captureLabels(pairs []*typesv1.LabelPair) map[string]string {
	selected := make(map[string]string, min(len(pairs), profiledump.MaxLabels))
	remaining := profiledump.MaxLabelBytes
	for _, pair := range pairs {
		name, value := pair.GetName(), pair.GetValue()
		if len(selected) == profiledump.MaxLabels {
			break
		}
		if len(name)+len(value) > remaining || profiledump.ValidateLabel(name, value) != nil {
			continue
		}
		if _, exists := selected[name]; exists {
			continue
		}
		selected[name] = value
		remaining -= len(name) + len(value)
	}
	return selected
}
