package otlp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/grafana/dskit/user"
	"google.golang.org/grpc"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	pprofileotlp "github.com/grafana/pyroscope/api/otlp/collector/profiles/v1experimental"
	v1 "github.com/grafana/pyroscope/api/otlp/common/v1"
	"github.com/grafana/pyroscope/api/otlp/profiles/v1experimental"
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
	Push(ctx context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error)
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

		// Get service name
		serviceName := getServiceNameFromAttributes(rp.Resource.GetAttributes())

		// Start with default labels
		labels := getDefaultLabels(serviceName)

		// Track processed attribute keys to avoid duplicates across levels
		processedKeys := make(map[string]bool)

		// Add resource attributes
		labels = appendAttributesUnique(labels, rp.Resource.GetAttributes(), processedKeys)

		sps := rp.ScopeProfiles
		for j := 0; j < len(sps); j++ {
			sp := sps[j]

			// Add scope attributes
			labels = appendAttributesUnique(labels, sp.Scope.GetAttributes(), processedKeys)

			for k := 0; k < len(sp.Profiles); k++ {
				p := sp.Profiles[k]

				// Add profile attributes
				labels = appendAttributesUnique(labels, p.GetAttributes(), processedKeys)

				// Add profile-specific attributes from samples/attributetable
				labels = appendProfileLabels(labels, p.Profile, processedKeys)

				pprofBytes, err := OprofToPprof(p.Profile)
				if err != nil {
					return &pprofileotlp.ExportProfilesServiceResponse{}, fmt.Errorf("failed to convert from OTLP to legacy pprof: %w", err)
				}

				_ = os.WriteFile(".tmp/elastic.pprof", pprofBytes, 0644)

				req := &pushv1.PushRequest{
					Series: []*pushv1.RawProfileSeries{
						{
							Labels: labels,
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
func getDefaultLabels(serviceName string) []*typesv1.LabelPair {
	return []*typesv1.LabelPair{
		{
			Name:  "__name__",
			Value: "process_cpu",
		},
		{
			Name:  "service_name",
			Value: serviceName,
		},
		{
			Name:  "__delta__",
			Value: "false",
		},
		{
			Name:  "pyroscope_spy",
			Value: "unknown",
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

func appendProfileLabels(labels []*typesv1.LabelPair, profile *v1experimental.Profile, processedKeys map[string]bool) []*typesv1.LabelPair {
	if profile == nil {
		return labels
	}

	// Create mapping of attribute indices to their values
	attrMap := make(map[uint64]v1.AnyValue)
	for i, attr := range profile.GetAttributeTable() {
		val := attr.GetValue()
		if val.GetValue() != nil {
			attrMap[uint64(i)] = val
		}
	}

	// Process only attributes referenced in samples
	for _, sample := range profile.Sample {
		for _, attrIdx := range sample.GetAttributes() {
			attr := profile.AttributeTable[attrIdx]
			// Skip if we've already processed this key at any level
			if processedKeys[attr.Key] {
				continue
			}

			if value, exists := attrMap[attrIdx]; exists {
				if sv := value.GetStringValue(); sv != "" {
					labels = append(labels, &typesv1.LabelPair{
						Name:  attr.Key,
						Value: sv,
					})
					processedKeys[attr.Key] = true
				}
			}
		}
	}

	return labels
}
