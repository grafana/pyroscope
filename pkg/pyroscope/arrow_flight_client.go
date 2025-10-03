package pyroscope

import (
	"fmt"
	"net"

	"github.com/go-kit/log"

	distributorflight "github.com/grafana/pyroscope/pkg/distributor/flight"
	"github.com/grafana/pyroscope/pkg/distributor/writepath"
)

// initArrowFlightSegmentWriterClient creates Arrow Flight segmentwriter client for v2 write path
func (f *Pyroscope) initArrowFlightSegmentWriterClient() (writepath.SegmentWriterClient, error) {
	logger := log.With(f.logger, "component", "arrow-flight-segmentwriter-client")

	// Always create Arrow Flight client in v2 mode for graceful fallback
	if !f.Cfg.V2 {
		logger.Log("msg", "Arrow Flight client not created: v2 not enabled")
		return nil, nil // Not an error - just not needed
	}

	// Get Arrow Flight configuration (defaults are used if not configured)
	config := f.Overrides.WritePathOverrides("")
	
	// Create client even if not explicitly enabled - router will handle graceful fallback
	if !config.ArrowFlight.Enabled {
		logger.Log("msg", "Arrow Flight client created with default fallback behavior", "enabled", false)
	}

	logger.Log("msg", "creating Arrow Flight segmentwriter client",
		"address", config.ArrowFlight.Address,
		"port", config.ArrowFlight.Port,
		"timeout", config.ArrowFlight.Timeout)

	// Create Arrow Flight client config
	flightConfig := distributorflight.FlightClientConfig{
		Address: net.JoinHostPort(config.ArrowFlight.Address, fmt.Sprintf("%d", config.ArrowFlight.Port)),
		Timeout: config.ArrowFlight.Timeout,
		TLS:     config.ArrowFlight.TLS,
	}

	// Create Arrow Flight segmentwriter client
	arrowClient, err := distributorflight.NewArrowFlightSegmentWriterClient(flightConfig, logger, f.reg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Arrow Flight segmentwriter client: %w", err)
	}

	logger.Log("msg", "Arrow Flight segmentwriter client created successfully")
	return arrowClient, nil
}
