package otlp

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/grafana/dskit/user"
	"google.golang.org/grpc"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	pprofileotlp "github.com/grafana/pyroscope/api/otlp/collector/profiles/v1development"
	v1 "github.com/grafana/pyroscope/api/otlp/common/v1"
	distirbutormodel "github.com/grafana/pyroscope/pkg/distributor/model"
	pyromodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/tenant"
)

type ingestHandler struct {
	pprofileotlp.UnimplementedProfilesServiceServer
	svc                 PushService
	log                 log.Logger
	handler             http.Handler
	multitenancyEnabled bool
}

type Handler interface {
	http.Handler
	pprofileotlp.ProfilesServiceServer
}

type PushService interface {
	PushParsed(ctx context.Context, req *distirbutormodel.PushRequest) (*connect.Response[pushv1.PushResponse], error)
}

func NewOTLPIngestHandler(svc PushService, l log.Logger, me bool) Handler {
	h := &ingestHandler{
		svc:                 svc,
		log:                 l,
		multitenancyEnabled: me,
	}

	grpcServer := grpc.NewServer()
	pprofileotlp.RegisterProfilesServiceServer(grpcServer, h)

	h.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
			return
		}

		// Handle HTTP requests (if we want to support HTTP/Protobuf in future)
		//if r.URL.Path == "/v1/profiles" {}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	return h
}

func (h *ingestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler.ServeHTTP(w, r)
}

func (h *ingestHandler) Export(ctx context.Context, er *pprofileotlp.ExportProfilesServiceRequest) (*pprofileotlp.ExportProfilesServiceResponse, error) {
	// TODO: @petethepig This logic is copied from util.AuthenticateUser and should be refactored into a common function
	// Extracts user ID from the request metadata and returns and injects the user ID in the context
	if !h.multitenancyEnabled {
		ctx = user.InjectOrgID(ctx, tenant.DefaultTenantID)
	} else {
		var err error
		_, ctx, err = user.ExtractFromGRPCRequest(ctx)
		if err != nil {
			level.Error(h.log).Log("msg", "failed to extract tenant ID from GRPC request", "err", err)
			return &pprofileotlp.ExportProfilesServiceResponse{}, fmt.Errorf("failed to extract tenant ID from GRPC request: %w", err)
		}
	}

	rps := er.ResourceProfiles
	for i := 0; i < len(rps); i++ {
		rp := rps[i]

		serviceName := getServiceNameFromAttributes(rp.Resource.GetAttributes())

		sps := rp.ScopeProfiles
		for j := 0; j < len(sps); j++ {
			sp := sps[j]

			for k := 0; k < len(sp.Profiles); k++ {
				p := sp.Profiles[k]

				pprofProfiles, err := ConvertOtelToGoogle(p)
				if err != nil {
					return &pprofileotlp.ExportProfilesServiceResponse{}, fmt.Errorf("failed to convert otel profile: %w", err)
				}

				req := &distirbutormodel.PushRequest{
					RawProfileSize: p.Size(),
					RawProfileType: distirbutormodel.RawProfileTypeOTEL,
				}

				for samplesServiceName, pprofProfile := range pprofProfiles {
					labels := getDefaultLabels()
					labels = append(labels, pprofProfile.name)
					processedKeys := map[string]bool{pyromodel.LabelNameProfileName: true}
					labels = appendAttributesUnique(labels, rp.Resource.GetAttributes(), processedKeys)
					labels = appendAttributesUnique(labels, sp.Scope.GetAttributes(), processedKeys)
					svc := samplesServiceName
					if svc == "" {
						svc = serviceName
					}
					labels = append(labels, &typesv1.LabelPair{
						Name:  pyromodel.LabelNameServiceName,
						Value: svc,
					})

					s := &distirbutormodel.ProfileSeries{
						Labels: labels,
						Samples: []*distirbutormodel.ProfileSample{
							{
								RawProfile: nil,
								Profile:    pprof.RawFromProto(pprofProfile.profile),
								ID:         uuid.New().String(),
							},
						},
					}
					req.Series = append(req.Series, s)
				}
				if len(req.Series) == 0 {
					continue
				}
				_, err = h.svc.PushParsed(ctx, req)
				if err != nil {
					h.log.Log("msg", "failed to push profile", "err", err)
					return &pprofileotlp.ExportProfilesServiceResponse{}, fmt.Errorf("failed to make a GRPC request: %w", err)
				}
			}
		}
	}

	return &pprofileotlp.ExportProfilesServiceResponse{}, nil
}

// getServiceNameFromAttributes extracts service name from OTLP resource attributes.
// Returns "unknown" if service name is not found or empty.
func getServiceNameFromAttributes(attrs []v1.KeyValue) string {
	for _, attr := range attrs {
		if attr.Key == "service.name" {
			val := attr.GetValue()
			if sv := val.GetStringValue(); sv != "" {
				return sv
			}
			break
		}
	}
	return "unknown"
}

// getDefaultLabels returns the required base labels for Pyroscope profiles
func getDefaultLabels() []*typesv1.LabelPair {
	return []*typesv1.LabelPair{
		{
			Name:  pyromodel.LabelNameDelta,
			Value: "false",
		},
		{
			Name:  pyromodel.LabelNameOTEL,
			Value: "true",
		},
	}
}

func appendAttributesUnique(labels []*typesv1.LabelPair, attrs []v1.KeyValue, processedKeys map[string]bool) []*typesv1.LabelPair {
	for _, attr := range attrs {
		// Skip if we've already seen this key at any level
		if processedKeys[attr.Key] {
			continue
		}

		val := attr.GetValue()
		if sv := val.GetStringValue(); sv != "" {
			labels = append(labels, &typesv1.LabelPair{
				Name:  attr.Key,
				Value: sv,
			})
			processedKeys[attr.Key] = true
		}
	}
	return labels
}
