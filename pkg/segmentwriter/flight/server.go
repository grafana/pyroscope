package flight

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"

	"github.com/apache/arrow/go/v18/arrow/flight"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/segmentwriter"
)

// FlightServer implements Arrow Flight server for segmentwriters
type FlightServer struct {
	logger log.Logger
	sw     *segmentwriter.SegmentWriterService
}

// NewFlightServer creates a new Arrow Flight server for segmentwriters
func NewFlightServer(logger log.Logger, sw *segmentwriter.SegmentWriterService, registerer prometheus.Registerer) *FlightServer {
	return &FlightServer{
		logger: logger,
		sw:     sw,
	}
}

// Run starts the Arrow Flight server
func (s *FlightServer) Run(ctx context.Context, addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	grpcServer := grpc.NewServer()

	// Register Flight service
	flight.RegisterFlightServiceServer(grpcServer, &flightServerWrapper{FlightServer: s})

	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()

	s.logger.Log("msg", "starting Arrow Flight server", "addr", addr)
	return grpcServer.Serve(listener)
}

// flightServerWrapper implements the complete FlightServer interface
type flightServerWrapper struct {
	flight.BaseFlightServer
	*FlightServer
}

func (w *flightServerWrapper) DoPut(stream flight.FlightService_DoPutServer) error {
	w.logger.Log("msg", "Received Arrow Flight DoPut request")

	// Read descriptor first
	_, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("failed to receive descriptor: %w", err)
	}

	// Read data batches - expecting 5 batches in order:
	// samples, locations, functions, mappings, strings
	var batches [][]byte
	for {
		recv, err := stream.Recv()
		if err != nil {
			if err != io.EOF {
				w.logger.Log("msg", "error receiving data", "error", err)
			}
			break
		}

		if len(recv.DataHeader) > 0 {
			batches = append(batches, recv.DataHeader)
		}
	}

	w.logger.Log("msg", "received Arrow Flight data", "batches", len(batches))

	// Log batch sizes for debugging
	for i, batch := range batches {
		w.logger.Log("msg", "batch details", "index", i, "size", len(batch))
	}

	// Extract metadata from context
	tenantID := extractTenantID(stream.Context())
	profileID := extractProfileID(stream.Context())
	labels := extractLabels(stream.Context())
	annotations := extractAnnotations(stream.Context())
	shard := extractShard(stream.Context())
	arrowMetadata := extractArrowMetadata(stream.Context())

	w.logger.Log("msg", "extracted metadata",
		"tenant", tenantID,
		"profile_id_len", len(profileID),
		"labels_count", len(labels),
		"shard", shard,
		"has_arrow_metadata", arrowMetadata != nil)

	// Reconstruct ArrowProfileData from batches
	// Expected order: samples, locations, functions, mappings, strings
	arrowProfile := &segmentwriterv1.ArrowProfileData{
		Metadata: arrowMetadata,
	}
	if len(batches) > 0 {
		arrowProfile.SamplesBatch = batches[0]
		w.logger.Log("msg", "set SamplesBatch", "size", len(batches[0]))
	} else {
		w.logger.Log("msg", "WARNING: no SamplesBatch received!")
	}
	if len(batches) > 1 {
		arrowProfile.LocationsBatch = batches[1]
		w.logger.Log("msg", "set LocationsBatch", "size", len(batches[1]))
	} else {
		w.logger.Log("msg", "WARNING: no LocationsBatch received!")
	}
	if len(batches) > 2 {
		arrowProfile.FunctionsBatch = batches[2]
	}
	if len(batches) > 3 {
		arrowProfile.MappingsBatch = batches[3]
	}
	if len(batches) > 4 {
		arrowProfile.StringsBatch = batches[4]
	} else {
		w.logger.Log("msg", "WARNING: no StringsBatch received!")
	}

	// Create push request
	req := &segmentwriterv1.PushRequest{
		TenantId:     tenantID,
		ProfileId:    profileID, // Already decoded to []byte
		Labels:       labels,
		Annotations:  annotations,
		Shard:        shard,
		ArrowProfile: arrowProfile,
	}

	// Call the segmentwriter service
	_, err = w.sw.Push(stream.Context(), req)
	if err != nil {
		return fmt.Errorf("failed to push to segmentwriter: %w", err)
	}

	// Send success response
	return stream.Send(&flight.PutResult{AppMetadata: []byte("success")})
}

// extractTenantID extracts tenant ID from Flight context metadata
func extractTenantID(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "anonymous"
	}

	if tenantIDs := md.Get("tenant-id"); len(tenantIDs) > 0 {
		return tenantIDs[0]
	}

	return "anonymous"
}

// extractProfileID extracts profile ID from Flight context metadata
// Returns the decoded binary UUID (16 bytes)
func extractProfileID(ctx context.Context) []byte {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil
	}

	if profileIDs := md.Get("profile-id"); len(profileIDs) > 0 {
		// Decode hex string back to binary
		profileID := profileIDs[0]
		if profileID == "" {
			return nil
		}
		// profileID is hex encoded, decode it
		decoded := make([]byte, len(profileID)/2)
		for i := 0; i < len(decoded); i++ {
			fmt.Sscanf(profileID[i*2:i*2+2], "%02x", &decoded[i])
		}
		return decoded
	}

	return nil
}

// extractLabels extracts labels from Flight context metadata
func extractLabels(ctx context.Context) []*typesv1.LabelPair {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil
	}

	var labels []*typesv1.LabelPair
	if labelValues := md.Get("labels"); len(labelValues) > 0 {
		// Deserialize from JSON
		if err := json.Unmarshal([]byte(labelValues[0]), &labels); err != nil {
			// Log error but don't fail - return empty labels
			return nil
		}
	}

	return labels
}

// extractAnnotations extracts annotations from Flight context metadata
func extractAnnotations(ctx context.Context) []*typesv1.ProfileAnnotation {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil
	}

	var annotations []*typesv1.ProfileAnnotation
	if annotationValues := md.Get("annotations"); len(annotationValues) > 0 {
		// Deserialize from JSON
		if err := json.Unmarshal([]byte(annotationValues[0]), &annotations); err != nil {
			// Log error but don't fail - return empty annotations
			return nil
		}
	}

	return annotations
}

// extractShard extracts shard from Flight context metadata
func extractShard(ctx context.Context) uint32 {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return 0
	}

	if shardValues := md.Get("shard"); len(shardValues) > 0 {
		// Parse shard value
		// For now return 0
	}

	return 0
}

// extractArrowMetadata extracts Arrow profile metadata from Flight context
func extractArrowMetadata(ctx context.Context) *segmentwriterv1.ProfileMetadata {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return &segmentwriterv1.ProfileMetadata{}
	}

	metadataValues := md.Get("arrow-metadata")
	if len(metadataValues) == 0 {
		return &segmentwriterv1.ProfileMetadata{}
	}

	// Deserialize full metadata from JSON
	// CRITICAL: This includes SampleType which is needed for Value array allocation!
	meta := &segmentwriterv1.ProfileMetadata{}
	if err := json.Unmarshal([]byte(metadataValues[0]), meta); err != nil {
		// Log error but return empty metadata rather than failing
		// TODO: proper error handling
		return &segmentwriterv1.ProfileMetadata{}
	}

	return meta
}
