package flight

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/apache/arrow/go/v18/arrow/flight"
	arrowflightpb "github.com/apache/arrow/go/v18/arrow/flight/gen/flight"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	"github.com/grafana/pyroscope/pkg/distributor/model"
	"github.com/grafana/pyroscope/pkg/tenant"
)

// FlightClientConfig configures the Arrow Flight client
type FlightClientConfig struct {
	Address     string        `yaml:"address"`
	Timeout     time.Duration `yaml:"timeout"`
	TLS         bool          `yaml:"tls"`
	TLSInsecure bool          `yaml:"tls_insecure"`
}

// FlightClient implements Arrow Flight client for sending profiles to segmentwriters
type FlightClient struct {
	config FlightClientConfig
	logger log.Logger
	client flight.Client

	// Metrics
	pushRequests   prometheus.Counter
	pushErrors     prometheus.Counter
	dataBytesOut   prometheus.Counter
	dataBatchesOut prometheus.Counter
	pushDuration   prometheus.Histogram
}

// NewFlightClient creates a new Arrow Flight client
func NewFlightClient(config FlightClientConfig, logger log.Logger, registerer prometheus.Registerer) (*FlightClient, error) {
	// Create gRPC client options
	var opts []grpc.DialOption
	if !config.TLS {
		opts = append(opts, grpc.WithInsecure())
	}

	// Connect to Arrow Flight server
	conn, err := grpc.Dial(config.Address, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Arrow Flight server at %s: %w", config.Address, err)
	}

	client := flight.NewClientFromConn(conn, nil)

	pushRequests := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "pyroscope",
		Subsystem: "distributor",
		Name:      "arrow_flight_push_requests_total",
		Help:      "Total number of Arrow Flight push requests sent",
	})
	pushErrors := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "pyroscope",
		Subsystem: "distributor",
		Name:      "arrow_flight_push_errors_total",
		Help:      "Total number of Arrow Flight push errors",
	})
	dataBytesOut := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "pyroscope",
		Subsystem: "distributor",
		Name:      "arrow_flight_data_bytes_out_total",
		Help:      "Total bytes of Arrow data sent via Flight",
	})
	dataBatchesOut := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "pyroscope",
		Subsystem: "distributor",
		Name:      "arrow_flight_data_batches_out_total",
		Help:      "Total number of Arrow record batches sent via Flight",
	})
	pushDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "pyroscope",
		Subsystem: "distributor",
		Name:      "arrow_flight_push_duration_seconds",
		Help:      "Duration of Arrow Flight push requests",
		Buckets:   prometheus.DefBuckets,
	})

	for _, metric := range []prometheus.Collector{pushRequests, pushErrors, dataBytesOut, dataBatchesOut, pushDuration} {
		registerer.MustRegister(metric)
	}

	return &FlightClient{
		config:         config,
		logger:         logger,
		client:         client,
		pushRequests:   pushRequests,
		pushErrors:     pushErrors,
		dataBytesOut:   dataBytesOut,
		dataBatchesOut: dataBatchesOut,
		pushDuration:   pushDuration,
	}, nil
}

// Push sends profile data to segmentwriter via Arrow Flight
func (c *FlightClient) Push(ctx context.Context, profileSeries *model.ProfileSeries) error {
	c.pushRequests.Inc()
	start := time.Now()
	defer func() {
		c.pushDuration.Observe(time.Since(start).Seconds())
	}()

	// Set timeout for the request
	if c.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.config.Timeout)
		defer cancel()
	}

	// Inject tenant ID into metadata
	ctx = tenant.InjectTenantID(ctx, profileSeries.TenantID)

	// Use the ProfileSeries directly as it contains the profile data

	// Convert profile to Arrow format
	arrowData, err := c.profileToArrow(profileSeries)
	if err != nil {
		c.pushErrors.Inc()
		return fmt.Errorf("failed to convert profile to Arrow format: %w", err)
	}

	// Flight descriptor not needed for DoPut in this version

	// Add metadata to headers
	md := map[string]string{
		"tenant-id":  profileSeries.TenantID,
		"profile-id": profileSeries.ID,
	}

	// Create metadata context
	for key, value := range md {
		ctx = metadata.AppendToOutgoingContext(ctx, key, value)
	}

	// Send data via Arrow Flight
	stream, err := c.client.DoPut(ctx)
	if err != nil {
		c.pushErrors.Inc()
		return fmt.Errorf("failed to create Flight stream: %w", err)
	}

	// First, send FlightDescriptor (required by Arrow Flight protocol)
	// Create a simple descriptor - the path identifies this as a push operation
	descriptor := &flight.FlightDescriptor{
		Path: []string{"pyroscope", "push"},
	}
	err = stream.Send(&flight.FlightData{
		FlightDescriptor: descriptor,
	})
	if err != nil {
		c.pushErrors.Inc()
		return fmt.Errorf("failed to send descriptor: %w", err)
	}

	// Send Arrow data
	totalBytes, err := c.sendArrowData(stream, arrowData)
	if err != nil {
		c.pushErrors.Inc()
		return fmt.Errorf("failed to send Arrow data: %w", err)
	}

	c.dataBytesOut.Add(float64(totalBytes))
	c.dataBatchesOut.Inc()

	// Signal we're done sending data
	err = stream.CloseSend()
	if err != nil {
		c.pushErrors.Inc()
		return fmt.Errorf("failed to close send stream: %w", err)
	}

	// Read response
	_, err = stream.Recv()
	if err != nil {
		c.pushErrors.Inc()
		return fmt.Errorf("failed to receive response: %w", err)
	}

	return nil
}

// sendArrowData sends Arrow format data over Flight stream
func (c *FlightClient) sendArrowData(stream flight.FlightService_DoPutClient, arrowData *segmentwriterv1.ArrowProfileData) (int, error) {
	// This would serialize the Arrow profile data and send it over the stream
	// For now, this is a placeholder implementation

	totalBytes := 0

	// Send samples batch
	if len(arrowData.SamplesBatch) > 0 {
		err := stream.Send(&flight.FlightData{
			DataHeader: arrowData.SamplesBatch,
		})
		if err != nil {
			return totalBytes, err
		}
		totalBytes += len(arrowData.SamplesBatch)
	}

	// Send other batches
	batches := [][]byte{
		arrowData.LocationsBatch,
		arrowData.FunctionsBatch,
		arrowData.MappingsBatch,
		arrowData.StringsBatch,
	}

	for _, batch := range batches {
		if len(batch) > 0 {
			err := stream.Send(&flight.FlightData{
				DataHeader: batch,
			})
			if err != nil {
				return totalBytes, err
			}
			totalBytes += len(batch)
		}
	}

	return totalBytes, nil
}

// profileToArrow converts a ProfileSeries to Arrow format
func (c *FlightClient) profileToArrow(series *model.ProfileSeries) (*segmentwriterv1.ArrowProfileData, error) {
	// This would use the existing Arrow conversion logic from pkg/distributor/arrow
	// For now, return a placeholder

	metadata := &segmentwriterv1.ProfileMetadata{
		TimeNanos:         time.Now().UnixNano(),
		DurationNanos:     0,
		Period:            0,
		DropFrames:        0,
		KeepFrames:        0,
		DefaultSampleType: 0,
		SampleType:        []*segmentwriterv1.ValueType{},
		PeriodType:        &segmentwriterv1.ValueType{},
		Comment:           []int64{},
	}

	return &segmentwriterv1.ArrowProfileData{
		Metadata:       metadata,
		SamplesBatch:   []byte("dummy-samples-data"), // Placeholder - would serialize actual Arrow record
		LocationsBatch: []byte{},
		FunctionsBatch: []byte{},
		MappingsBatch:  []byte{},
		StringsBatch:   []byte{},
	}, nil
}

// Close closes the Flight client connection
func (c *FlightClient) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// ArrowFlightSegmentWriterClient implements SegmentWriterClient using Arrow Flight
type ArrowFlightSegmentWriterClient struct {
	flightClient *FlightClient
	logger       log.Logger
	registry     prometheus.Registerer
}

// NewArrowFlightSegmentWriterClient creates a new Arrow Flight segmentwriter client
func NewArrowFlightSegmentWriterClient(config FlightClientConfig, logger log.Logger, registerer prometheus.Registerer) (*ArrowFlightSegmentWriterClient, error) {
	flightClient, err := NewFlightClient(config, logger, registerer)
	if err != nil {
		return nil, fmt.Errorf("failed to create Arrow Flight client: %w", err)
	}

	return &ArrowFlightSegmentWriterClient{
		flightClient: flightClient,
		logger:       logger,
		registry:     registerer,
	}, nil
}

// Push implements writepath.SegmentWriterClient interface using Arrow Flight
func (c *ArrowFlightSegmentWriterClient) Push(ctx context.Context, req *segmentwriterv1.PushRequest) (*segmentwriterv1.PushResponse, error) {
	c.logger.Log("msg", "sending push request via Arrow Flight", "tenant", req.TenantId)

	// If we have ArrowProfileData, send it directly via Arrow Flight
	if req.ArrowProfile != nil {
		err := c.sendArrowProfile(ctx, req)
		if err != nil {
			c.logger.Log("msg", "failed to send Arrow profile via Flight", "error", err)
			return nil, fmt.Errorf("failed to send Arrow profile: %w", err)
		}
		c.logger.Log("msg", "successfully sent Arrow profile via Flight")
		return &segmentwriterv1.PushResponse{}, nil
	}

	// Fallback: convert to ProfileSeries and use our Flight client
	// This shouldn't happen in v2 write path, but good to have as fallback
	profileSeries := c.convertPushRequestToProfileSeries(req)

	err := c.flightClient.Push(ctx, profileSeries)
	if err != nil {
		return nil, fmt.Errorf("failed to push profile series: %w", err)
	}

	return &segmentwriterv1.PushResponse{}, nil
}

// sendArrowProfile sends Arrow profile data directly via Arrow Flight
func (c *ArrowFlightSegmentWriterClient) sendArrowProfile(ctx context.Context, req *segmentwriterv1.PushRequest) error {
	// Create Arrow batches from PushRequest
	var samplesBatch, locationsBatch, functionsBatch, mappingsBatch, stringsBatch []byte

	if req.ArrowProfile.SamplesBatch != nil {
		samplesBatch = req.ArrowProfile.SamplesBatch
	}

	if req.ArrowProfile.LocationsBatch != nil {
		locationsBatch = req.ArrowProfile.LocationsBatch
	}

	if req.ArrowProfile.FunctionsBatch != nil {
		functionsBatch = req.ArrowProfile.FunctionsBatch
	}

	if req.ArrowProfile.MappingsBatch != nil {
		mappingsBatch = req.ArrowProfile.MappingsBatch
	}

	if req.ArrowProfile.StringsBatch != nil {
		stringsBatch = req.ArrowProfile.StringsBatch
	}

	// Add metadata to context
	// Encode ProfileId as hex string for gRPC metadata (must be printable ASCII)
	profileIDStr := fmt.Sprintf("%x", req.ProfileId)

	// Also send the Arrow profile metadata
	var metadataJSON string
	if req.ArrowProfile.Metadata != nil {
		// Serialize metadata as a simple string for transport
		// In production, use proto Marshal or JSON
		metadataJSON = fmt.Sprintf("time=%d,duration=%d,period=%d",
			req.ArrowProfile.Metadata.TimeNanos,
			req.ArrowProfile.Metadata.DurationNanos,
			req.ArrowProfile.Metadata.Period)
	}

	c.logger.Log("msg", "adding metadata to Arrow Flight request",
		"tenant", req.TenantId,
		"profile_id_len", len(req.ProfileId),
		"profile_id_hex_len", len(profileIDStr),
		"shard", req.Shard,
		"has_arrow_metadata", req.ArrowProfile.Metadata != nil)

	ctx = metadata.AppendToOutgoingContext(ctx,
		"tenant-id", req.TenantId,
		"profile-id", profileIDStr,
		"shard", fmt.Sprintf("%d", req.Shard),
		"arrow-metadata", metadataJSON)

	// Send Arrow Flight DoPut request
	return c.sendArrowFlightData(ctx, req.TenantId, samplesBatch, locationsBatch, functionsBatch, mappingsBatch, stringsBatch)
}

// sendArrowFlightData sends Arrow data via Flight DoPut
func (c *ArrowFlightSegmentWriterClient) sendArrowFlightData(ctx context.Context, tenantID string, samplesBatch, locationsBatch, functionsBatch, mappingsBatch, stringsBatch []byte) error {
	stream, err := c.flightClient.client.DoPut(ctx)
	if err != nil {
		return fmt.Errorf("failed to create DoPut stream: %w", err)
	}

	// Send descriptor first message
	err = stream.Send(&arrowflightpb.FlightData{
		DataHeader: []byte("descriptor"),
	})
	if err != nil {
		return fmt.Errorf("failed to send descriptor: %w", err)
	}

	// Send data batches
	batches := [][]byte{
		samplesBatch,
		locationsBatch,
		functionsBatch,
		mappingsBatch,
		stringsBatch,
	}

	for i, batch := range batches {
		if len(batch) > 0 {
			err = stream.Send(&arrowflightpb.FlightData{
				DataHeader: batch,
			})
			if err != nil {
				return fmt.Errorf("failed to send batch %d: %w", i, err)
			}
		}
	}

	// Close the send side to signal we're done sending
	err = stream.CloseSend()
	if err != nil {
		return fmt.Errorf("failed to close send stream: %w", err)
	}

	// Now receive the response
	_, err = stream.Recv()
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to receive response: %w", err)
	}

	return nil
}

// convertPushRequestToProfileSeries converts a PushRequest to ProfileSeries
func (c *ArrowFlightSegmentWriterClient) convertPushRequestToProfileSeries(req *segmentwriterv1.PushRequest) *model.ProfileSeries {
	return &model.ProfileSeries{
		TenantID:    req.TenantId,
		Labels:      req.Labels,
		Profile:     nil, // Would convert from ArrowProfile
		ID:          string(req.ProfileId),
		Annotations: req.Annotations,
	}
}

// Close closes the Arrow Flight segmentwriter client
func (c *ArrowFlightSegmentWriterClient) Close() error {
	if c.flightClient != nil {
		return c.flightClient.Close()
	}
	return nil
}
