package flight

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/apache/arrow/go/v18/arrow/flight"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	"github.com/grafana/pyroscope/pkg/distributor/model"
	"github.com/grafana/pyroscope/pkg/tenant"
)

// SimpleFlightClientConfig configures the Arrow Flight client
type SimpleFlightClientConfig struct {
	Address string
	Timeout time.Duration
	TLS     bool
}

// SimpleFlightClient implements Arrow Flight client for testing
type SimpleFlightClient struct {
	config SimpleFlightClientConfig
	logger *log.Logger
	client flight.Client

	// Metrics
	pushRequests prometheus.Counter
	pushErrors   prometheus.Counter
}

// NewSimpleFlightClient creates a new Simple Arrow Flight client
func NewSimpleFlightClient(config SimpleFlightClientConfig, logger *log.Logger) (*SimpleFlightClient, error) {
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
		Subsystem: "flight_test",
		Name:      "push_requests_total",
		Help:      "Total number of Flight push requests sent",
	})
	pushErrors := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "pyroscope",
		Subsystem: "flight_test",
		Name:      "push_errors_total",
		Help:      "Total number of Flight push errors",
	})

	registerer := prometheus.NewRegistry()
	registerer.MustRegister(pushRequests, pushErrors)

	return &SimpleFlightClient{
		config:       config,
		logger:       logger,
		client:       client,
		pushRequests: pushRequests,
		pushErrors:   pushErrors,
	}, nil
}

// Push sends profile data to segmentwriter via Arrow Flight
func (c *SimpleFlightClient) Push(ctx context.Context, profileSeries *model.ProfileSeries) error {
	c.pushRequests.Inc()

	// Set timeout for the request
	if c.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.config.Timeout)
		defer cancel()
	}

	// Inject tenant ID into metadata
	ctx = tenant.InjectTenantID(ctx, profileSeries.TenantID)

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
		return fmt.Errorf("failed to/create Flight stream: %w", err)
	}
	defer stream.CloseSend()

	// Create test Arrow data
	arrowData := &segmentwriterv1.ArrowProfileData{
		Metadata: &segmentwriterv1.ProfileMetadata{
			TimeNanos: time.Now().UnixNano(),
		},
		SamplesBatch:   []byte("test_samples_data"),
		LocationsBatch: []byte("test_locations_data"),
		StringsBatch:   []byte("test_strings_data"),
	}

	// Send Arrow data
	totalBytes := 0
	batches := [][]byte{
		arrowData.SamplesBatch,
		arrowData.LocationsBatch,
		arrowData.StringsBatch,
	}

	for _, batch := range batches {
		if len(batch) > 0 {
			err := stream.Send(&flight.FlightData{
				DataHeader: batch,
			})
			if err != nil {
				c.pushErrors.Inc()
				return fmt.Errorf("failed to send data: %w", err)
			}
			totalBytes += len(batch)
		}
	}

	c.logger.Printf("Sent %d bytes via Arrow Flight", totalBytes)

	// Read response
	_, err = stream.Recv()
	if err != nil {
		c.pushErrors.Inc()
		return fmt.Errorf("failed to receive response: %w", err)
	}

	return nil
}

// Close closes the Flight client connection
func (c *SimpleFlightClient) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

