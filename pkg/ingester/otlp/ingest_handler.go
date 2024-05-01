package otlp

import (
	"connectrpc.com/connect"
	"context"
	"fmt"
	"net/http"
	"os"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/tenant"

	pprofileotlp "github.com/grafana/pyroscope/api/otlp/collector/profiles/v1experimental"
)

type ingestHandler struct {
	pprofileotlp.UnimplementedProfilesServiceServer
	svc PushService
	log log.Logger
}

// TODO(@petethepig): split http and grpc
type Handler interface {
	http.Handler
	pprofileotlp.ProfilesServiceServer
}

type PushService interface {
	Push(ctx context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error)
}

func NewOTLPIngestHandler(svc PushService, l log.Logger) Handler {
	return &ingestHandler{
		svc: svc,
		log: level.Error(l),
	}
}

// TODO(@petethepig): implement
// TODO(@petethepig): split http and grpc
func (h *ingestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	panic("not implemented")

	req := &pushv1.PushRequest{}
	_, err := h.svc.Push(r.Context(), connect.NewRequest(req))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error: " + err.Error()))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// TODO(@petethepig): split http and grpc
func (h *ingestHandler) Export(ctx context.Context, er *pprofileotlp.ExportProfilesServiceRequest) (*pprofileotlp.ExportProfilesServiceResponse, error) {

	// TODO(@petethepig): make it tenant-aware
	ctx = tenant.InjectTenantID(ctx, tenant.DefaultTenantID)

	h.log.Log("msg", "Export called")

	rps := er.ResourceProfiles
	for i := 0; i < len(rps); i++ {
		rp := rps[i]

		labelsDst := []*typesv1.LabelPair{}
		// TODO(@petethepig): make labels work
		labelsDst = append(labelsDst, &typesv1.LabelPair{
			Name:  "__name__",
			Value: "process_cpu",
		})
		labelsDst = append(labelsDst, &typesv1.LabelPair{
			Name:  "service_name",
			Value: "otlp_test_app4",
		})
		labelsDst = append(labelsDst, &typesv1.LabelPair{
			Name:  "__delta__",
			Value: "false",
		})
		labelsDst = append(labelsDst, &typesv1.LabelPair{
			Name:  "pyroscope_spy",
			Value: "unknown",
		})

		sps := rp.ScopeProfiles
		for j := 0; j < len(sps); j++ {
			sp := sps[j]
			for k := 0; k < len(sp.Profiles); k++ {
				p := sp.Profiles[k]

				pprofBytes, err := OprofToPprof(p.Profile)
				if err != nil {
					return &pprofileotlp.ExportProfilesServiceResponse{}, fmt.Errorf("failed to convert from OTLP to legacy pprof: %w", err)
				}

				os.WriteFile(".tmp/elastic.pprof", pprofBytes, 0644)

				req := &pushv1.PushRequest{
					Series: []*pushv1.RawProfileSeries{
						{
							Labels: labelsDst,
							Samples: []*pushv1.RawSample{{
								RawProfile: pprofBytes,
								ID:         uuid.New().String(),
							}},
						},
					},
				}
				_, err = h.svc.Push(ctx, connect.NewRequest(req))
				if err != nil {
					h.log.Log("msg", "failed to push profile", "err", err)
					return &pprofileotlp.ExportProfilesServiceResponse{}, fmt.Errorf("failed to make a GRPC request: %w", err)
				}
			}
		}

	}

	return &pprofileotlp.ExportProfilesServiceResponse{}, nil
}
