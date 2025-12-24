package otlp

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/dustin/go-humanize"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/grafana/dskit/server"
	"github.com/grafana/dskit/tenant"
	pprofileotlp "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
	v1 "go.opentelemetry.io/proto/otlp/common/v1"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	distributormodel "github.com/grafana/pyroscope/pkg/distributor/model"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/pprof"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
	"github.com/grafana/pyroscope/pkg/validation"
)

type ingestHandler struct {
	pprofileotlp.UnimplementedProfilesServiceServer
	svc     PushService
	log     log.Logger
	handler http.Handler
	limits  Limits
}

type Handler interface {
	http.Handler
	pprofileotlp.ProfilesServiceServer
}

type PushService interface {
	PushBatch(ctx context.Context, req *distributormodel.PushRequest) error
}

type Limits interface {
	IngestionBodyLimitBytes(tenantID string) int64
}

func NewOTLPIngestHandler(cfg server.Config, svc PushService, l log.Logger, limits Limits) Handler {
	h := &ingestHandler{
		svc:    svc,
		log:    l,
		limits: limits,
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

func isHTTPRequestBodyTooLarge(err error) error {
	herr := new(http.MaxBytesError)
	if errors.As(err, &herr) {
		return validation.NewErrorf(validation.BodySizeLimit, "profile payload size exceeds limit of %s", humanize.Bytes(uint64(herr.Limit)))
	}
	return nil
}

func isKnownValidationError(err error) bool {
	return validation.ReasonOf(err) != validation.Unknown
}

func (h *ingestHandler) handleHTTPRequest(w http.ResponseWriter, r *http.Request) {
	tenantID, err := tenant.TenantID(r.Context())
	if err != nil {
		httputil.ErrorWithStatus(w, err, http.StatusUnauthorized)
		return
	}
	maxBodyBytes := h.limits.IngestionBodyLimitBytes(tenantID)

	defer r.Body.Close()

	var (
		errMsgBodyRead = "failed to read request body"
		reader         = r.Body
	)

	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		gzipReader, gzipErr := gzip.NewReader(r.Body)
		if gzipErr != nil {
			level.Error(h.log).Log("msg", "failed to create gzip reader", "err", gzipErr)
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		defer gzipReader.Close()
		errMsgBodyRead = "failed to read gzip-compressed request body"

		reader = gzipReader
		// Limit after decompression size
		if maxBodyBytes > 0 {
			reader = io.NopCloser(io.LimitReader(reader, maxBodyBytes+1))
		}
	}

	body, err := io.ReadAll(reader)
	if maxBodyBytes > 0 && len(body) == int(maxBodyBytes)+1 {
		err := validation.NewErrorf(validation.BodySizeLimit, "uncompressed profile payload size exceeds limit of %s", humanize.Bytes(uint64(maxBodyBytes)))
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		return
	}
	if err != nil {
		level.Error(h.log).Log("msg", errMsgBodyRead, "err", err)
		// handle if body size limit is hit with correct status code
		if herr := isHTTPRequestBodyTooLarge(err); herr != nil {
			http.Error(w, herr.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, errMsgBodyRead, http.StatusBadRequest)
		return
	}

	req := &pprofileotlp.ExportProfilesServiceRequest{}

	isJSONRequest := r.Header.Get("Content-Type") == "application/json"
	if isJSONRequest {
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

	resp, err := h.export(r.Context(), req)
	if err != nil {
		level.Error(h.log).Log("msg", "failed to process profiles", "err", err)
		if isKnownValidationError(err) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	respBytes, err := proto.Marshal(resp)
	if err != nil {
		level.Error(h.log).Log("msg", "failed to marshal response", "err", err)
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(respBytes); err != nil {
		level.Error(h.log).Log("msg", "failed to write response", "err", err)
	}
}

func (h *ingestHandler) Export(ctx context.Context, er *pprofileotlp.ExportProfilesServiceRequest) (*pprofileotlp.ExportProfilesServiceResponse, error) {
	return h.export(ctx, er)
}

func (h *ingestHandler) export(ctx context.Context, er *pprofileotlp.ExportProfilesServiceRequest) (*pprofileotlp.ExportProfilesServiceResponse, error) {
	_, err := tenant.TenantID(ctx)
	if err != nil {
		return &pprofileotlp.ExportProfilesServiceResponse{}, status.Errorf(codes.Unauthenticated, "failed to extract tenant ID from context: %s", err.Error())
	}

	dc := er.Dictionary
	if dc == nil {
		return &pprofileotlp.ExportProfilesServiceResponse{}, status.Errorf(codes.InvalidArgument, "missing profile metadata dictionary")
	}

	rps := er.ResourceProfiles
	if rps == nil {
		return &pprofileotlp.ExportProfilesServiceResponse{}, status.Errorf(codes.InvalidArgument, "missing resource profiles")
	}

	for _, rp := range rps {
		serviceName := getServiceNameFromAttributes(rp.Resource.GetAttributes())
		for _, sp := range rp.ScopeProfiles {
			for _, p := range sp.Profiles {
				pprofProfiles, err := ConvertOtelToGoogle(p, dc)
				if err != nil {
					grpcError := status.Errorf(codes.InvalidArgument, "failed to convert otel profile: %s", err.Error())
					return &pprofileotlp.ExportProfilesServiceResponse{}, grpcError
				}

				req := &distributormodel.PushRequest{
					ReceivedCompressedProfileSize: proto.Size(p),
					RawProfileType:                distributormodel.RawProfileTypeOTEL,
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

					s := &distributormodel.ProfileSeries{
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
