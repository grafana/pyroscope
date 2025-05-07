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
	pprofileotlp "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
	v1 "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	distirbutormodel "github.com/grafana/pyroscope/pkg/distributor/model"
	"github.com/grafana/pyroscope/pkg/model"
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
					RawProfileSize: proto.Size(p),
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
// according to otel spec https://github.com/open-telemetry/opentelemetry-go/blob/ecfb73581f1b05af85fc393c3ce996a90cf2a5e2/semconv/v1.30.0/attribute_group.go#L10011-L10025
// Returns "unknown_service:$process_name" if no service.name, but there is a process.executable.name
// Returns "unknown_service" if no service.name and no process.executable.name
func getServiceNameFromAttributes(attrs []*v1.KeyValue) string {
	fallback := model.AttrServiceNameFallback
	for _, attr := range attrs {
		if attr.Key == string(model.AttrServiceName) {
			val := attr.GetValue()
			if sv := val.GetStringValue(); sv != "" {
				return sv
			}
		}
		if attr.Key == string(model.AttrProcessExecutableName) {
			val := attr.GetValue()
			if sv := val.GetStringValue(); sv != "" {
				fallback += ":" + sv
			}
		}

	}
	return fallback
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

func appendAttributesUnique(labels []*typesv1.LabelPair, attrs []*v1.KeyValue, processedKeys map[string]bool) []*typesv1.LabelPair {
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
