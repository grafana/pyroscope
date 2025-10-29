package otlp

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/grafana/dskit/server"
	"github.com/grafana/dskit/user"
	pprofileotlp "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
	v1 "go.opentelemetry.io/proto/otlp/common/v1"

	"google.golang.org/grpc/status"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	distirbutormodel "github.com/grafana/pyroscope/pkg/distributor/model"
	"github.com/grafana/pyroscope/pkg/model"
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
	PushBatch(ctx context.Context, req *distirbutormodel.PushRequest) error
}

func NewOTLPIngestHandler(cfg server.Config, svc PushService, l log.Logger, multitenancyEnabled bool) Handler {
	h := &ingestHandler{
		svc:                 svc,
		log:                 l,
		multitenancyEnabled: multitenancyEnabled,
	}

	grpcServer := newGrpcServer(cfg)
	pprofileotlp.RegisterProfilesServiceServer(grpcServer, h)

	h.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
			return
		}

		// Handle HTTP/JSON and HTTP/Protobuf requests
		contentType := r.Header.Get("Content-Type")
		if contentType == "application/json" || contentType == "application/x-protobuf" || contentType == "application/protobuf" {
			h.handleHTTPRequest(w, r)
			return
		}

		http.Error(w, fmt.Sprintf("Unsupported Content-Type: %s", contentType), http.StatusUnsupportedMediaType)
	})

	return h
}

func newGrpcServer(cfg server.Config) *grpc.Server {
	grpcKeepAliveOptions := keepalive.ServerParameters{
		MaxConnectionIdle:     cfg.GRPCServerMaxConnectionIdle,
		MaxConnectionAge:      cfg.GRPCServerMaxConnectionAge,
		MaxConnectionAgeGrace: cfg.GRPCServerMaxConnectionAgeGrace,
		Time:                  cfg.GRPCServerTime,
		Timeout:               cfg.GRPCServerTimeout,
	}

	grpcKeepAliveEnforcementPolicy := keepalive.EnforcementPolicy{
		MinTime:             cfg.GRPCServerMinTimeBetweenPings,
		PermitWithoutStream: cfg.GRPCServerPingWithoutStreamAllowed,
	}

	grpcOptions := []grpc.ServerOption{
		grpc.KeepaliveParams(grpcKeepAliveOptions),
		grpc.KeepaliveEnforcementPolicy(grpcKeepAliveEnforcementPolicy),
		grpc.MaxRecvMsgSize(cfg.GRPCServerMaxRecvMsgSize),
		grpc.MaxSendMsgSize(cfg.GRPCServerMaxSendMsgSize),
		grpc.MaxConcurrentStreams(uint32(cfg.GRPCServerMaxConcurrentStreams)),
		grpc.NumStreamWorkers(uint32(cfg.GRPCServerNumWorkers)),
	}

	grpcOptions = append(grpcOptions, cfg.GRPCOptions...)

	return grpc.NewServer(grpcOptions...)
}

func (h *ingestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler.ServeHTTP(w, r)
}

// extractTenantIDFromHTTPRequest extracts the tenant ID from HTTP request headers.
// If multitenancy is disabled, it injects the default tenant ID.
// Returns a context with the tenant ID injected.
func (h *ingestHandler) extractTenantIDFromHTTPRequest(r *http.Request) (context.Context, error) {
	if !h.multitenancyEnabled {
		return user.InjectOrgID(r.Context(), tenant.DefaultTenantID), nil
	}

	_, ctx, err := user.ExtractOrgIDFromHTTPRequest(r)
	if err != nil {
		return nil, err
	}
	return ctx, nil
}

// extractTenantIDFromGRPCRequest extracts the tenant ID from a gRPC request context.
// If multitenancy is disabled, it injects the default tenant ID.
// Returns a context with the tenant ID injected.
func (h *ingestHandler) extractTenantIDFromGRPCRequest(ctx context.Context) (context.Context, error) {
	// TODO: ideally should be merged with function above
	if !h.multitenancyEnabled {
		return user.InjectOrgID(ctx, tenant.DefaultTenantID), nil
	}

	_, ctx, err := user.ExtractFromGRPCRequest(ctx)
	if err != nil {
		return nil, err
	}
	return ctx, nil
}

func (h *ingestHandler) handleHTTPRequest(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	ctx, err := h.extractTenantIDFromHTTPRequest(r)
	if err != nil {
		level.Error(h.log).Log("msg", "failed to extract tenant ID from HTTP request", "err", err)
		http.Error(w, "Failed to extract tenant ID from HTTP request", http.StatusUnauthorized)
		return
	}

	// Read the request body - we need to read it all for protobuf unmarshaling
	// Note: Protobuf wire format requires reading the entire message to determine field boundaries
	var body []byte

	// Check if the body is gzip-encoded
	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		gzipReader, gzipErr := gzip.NewReader(r.Body)
		if gzipErr != nil {
			level.Error(h.log).Log("msg", "failed to create gzip reader", "err", gzipErr)
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		defer gzipReader.Close()

		var readErr error
		body, readErr = io.ReadAll(gzipReader)
		if readErr != nil {
			level.Error(h.log).Log("msg", "failed to read gzip-compressed request body", "err", readErr)
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
	} else {
		var readErr error
		body, readErr = io.ReadAll(r.Body)
		if readErr != nil {
			level.Error(h.log).Log("msg", "failed to read request body", "err", readErr)
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
	}

	// Unmarshal the protobuf request
	req := &pprofileotlp.ExportProfilesServiceRequest{}

	if r.Header.Get("Content-Type") == "application/json" {
		if err := protojson.Unmarshal(body, req); err != nil {
			level.Error(h.log).Log("msg", "failed to unmarshal JSON request", "err", err)
			http.Error(w, "Failed to parse JSON request", http.StatusBadRequest)
			return
		}
	} else {
		if err := proto.Unmarshal(body, req); err != nil {
			level.Error(h.log).Log("msg", "failed to unmarshal protobuf request", "err", err)
			http.Error(w, "Failed to parse protobuf request", http.StatusBadRequest)
			return
		}
	}

	// Process the request using the existing export method
	resp, err := h.export(ctx, req)
	if err != nil {
		level.Error(h.log).Log("msg", "failed to process profiles", "err", err)
		// Convert gRPC status to HTTP status
		st, ok := status.FromError(err)
		if ok {
			switch st.Code() {
			case codes.InvalidArgument:
				http.Error(w, st.Message(), http.StatusBadRequest)
			case codes.Unauthenticated:
				http.Error(w, st.Message(), http.StatusUnauthorized)
			case codes.PermissionDenied:
				http.Error(w, st.Message(), http.StatusForbidden)
			default:
				http.Error(w, st.Message(), http.StatusInternalServerError)
			}
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Marshal the response
	respBytes, err := proto.Marshal(resp)
	if err != nil {
		level.Error(h.log).Log("msg", "failed to marshal response", "err", err)
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
		return
	}

	// Write the response
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(respBytes); err != nil {
		level.Error(h.log).Log("msg", "failed to write response", "err", err)
	}
}

// Export is the gRPC handler for the ProfilesService Export RPC.
// Extracts tenant ID from gRPC request metadata before processing.
func (h *ingestHandler) Export(ctx context.Context, er *pprofileotlp.ExportProfilesServiceRequest) (*pprofileotlp.ExportProfilesServiceResponse, error) {
	// Extract tenant ID from gRPC request
	ctx, err := h.extractTenantIDFromGRPCRequest(ctx)
	if err != nil {
		level.Error(h.log).Log("msg", "failed to extract tenant ID from gRPC request", "err", err)
		return &pprofileotlp.ExportProfilesServiceResponse{}, fmt.Errorf("failed to extract tenant ID from gRPC request: %w", err)
	}

	return h.export(ctx, er)
}

// export is the common implementation for processing OTLP profile export requests.
// The context must already have the tenant ID injected before calling this method.
func (h *ingestHandler) export(ctx context.Context, er *pprofileotlp.ExportProfilesServiceRequest) (*pprofileotlp.ExportProfilesServiceResponse, error) {

	dc := er.Dictionary
	if dc == nil {
		return &pprofileotlp.ExportProfilesServiceResponse{}, status.Errorf(codes.InvalidArgument, "missing profile metadata dictionary")
	}

	rps := er.ResourceProfiles
	if rps == nil {
		return &pprofileotlp.ExportProfilesServiceResponse{}, status.Errorf(codes.InvalidArgument, "missing resource profiles")
	}

	for i := 0; i < len(rps); i++ {
		rp := rps[i]

		serviceName := getServiceNameFromAttributes(rp.Resource.GetAttributes())

		sps := rp.ScopeProfiles
		for j := 0; j < len(sps); j++ {
			sp := sps[j]

			for k := 0; k < len(sp.Profiles); k++ {
				p := sp.Profiles[k]

				pprofProfiles, err := ConvertOtelToGoogle(p, dc)
				if err != nil {
					grpcError := status.Errorf(codes.InvalidArgument, "failed to convert otel profile: %s", err.Error())
					return &pprofileotlp.ExportProfilesServiceResponse{}, grpcError
				}

				req := &distirbutormodel.PushRequest{
					ReceivedCompressedProfileSize: proto.Size(p),
					RawProfileType:                distirbutormodel.RawProfileTypeOTEL,
				}

				for samplesServiceName, pprofProfile := range pprofProfiles {
					labels := getDefaultLabels()
					labels = append(labels, pprofProfile.name)
					processedKeys := map[string]bool{model.LabelNameProfileName: true}
					labels = appendAttributesUnique(labels, rp.Resource.GetAttributes(), processedKeys)
					labels = appendAttributesUnique(labels, sp.Scope.GetAttributes(), processedKeys)
					svc := samplesServiceName
					if svc == "" {
						svc = serviceName
					}
					labels = append(labels, &typesv1.LabelPair{
						Name:  model.LabelNameServiceName,
						Value: svc,
					})

					s := &distirbutormodel.ProfileSeries{
						Labels:     labels,
						RawProfile: nil,
						Profile:    pprof.RawFromProto(pprofProfile.profile),
						ID:         uuid.New().String(),
					}
					req.Series = append(req.Series, s)
				}
				if len(req.Series) == 0 {
					continue
				}
				err = h.svc.PushBatch(ctx, req)
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
			Name:  model.LabelNameDelta,
			Value: "false",
		},
		{
			Name:  model.LabelNameOTEL,
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
