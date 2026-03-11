package mcp

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prometheus/common/version"

	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
)

const defaultShutdownTimeout = 5 * time.Second

// Server is the MCP server service that implements dskit services.Service.
type Server struct {
	services.Service

	cfg           Config
	logger        log.Logger
	mcpServer     *server.MCPServer
	tools         *Tools
	querierClient querierv1connect.QuerierServiceClient

	// For stdio mode
	stdioServer *server.StdioServer
	stdioCancel context.CancelFunc

	// For SSE mode
	sseServer *server.SSEServer
}

// NewServer creates a new MCP server.
func NewServer(cfg Config, logger log.Logger, querierClient querierv1connect.QuerierServiceClient) (*Server, error) {
	s := &Server{
		cfg:           cfg,
		logger:        logger,
		querierClient: querierClient,
	}

	s.Service = services.NewBasicService(s.starting, s.running, s.stopping)
	return s, nil
}

func (s *Server) starting(ctx context.Context) error {
	// Create the MCP server
	s.mcpServer = server.NewMCPServer(
		"pyroscope",
		version.Version,
		server.WithInstructions(`
This server provides access to Pyroscope profiling data.

Available Capabilities:
- Fetch profiles: Query and retrieve profiling data in DOT format for analysis.

Use the fetch_pyroscope_profile tool to query profiles by specifying:
- profile_type: The type of profile (e.g., "process_cpu:cpu:nanoseconds:cpu:nanoseconds")
- matchers: Label selectors to filter profiles (e.g., {service_name="myapp"})
- time range: Optional start and end times in RFC3339 format
`),
	)

	// Create and register tools
	s.tools = NewTools(s.querierClient)
	s.tools.RegisterTools(s.mcpServer)

	level.Info(s.logger).Log("msg", "MCP server initialized", "transport", s.cfg.Transport)
	return nil
}

func (s *Server) running(ctx context.Context) error {
	switch s.cfg.Transport {
	case TransportStdio:
		return s.runStdio(ctx)
	case TransportSSE:
		// SSE mode is handled via HTTP handler registration
		// Just wait for context cancellation
		<-ctx.Done()
		return nil
	default:
		level.Error(s.logger).Log("msg", "unknown MCP transport", "transport", s.cfg.Transport)
		<-ctx.Done()
		return nil
	}
}

func (s *Server) runStdio(ctx context.Context) error {
	stdioCtx, cancel := context.WithCancel(ctx)
	s.stdioCancel = cancel

	s.stdioServer = server.NewStdioServer(s.mcpServer)

	level.Info(s.logger).Log("msg", "starting MCP server in stdio mode")

	// Run stdio server - this blocks until context is cancelled or stdin is closed
	err := s.stdioServer.Listen(stdioCtx, os.Stdin, os.Stdout)
	if err != nil && err != context.Canceled {
		level.Error(s.logger).Log("msg", "MCP stdio server error", "err", err)
		return err
	}

	return nil
}

func (s *Server) stopping(_ error) error {
	level.Info(s.logger).Log("msg", "stopping MCP server")

	if s.stdioCancel != nil {
		s.stdioCancel()
	}

	if s.sseServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
		defer cancel()
		if err := s.sseServer.Shutdown(shutdownCtx); err != nil {
			level.Error(s.logger).Log("msg", "error shutting down SSE server", "err", err)
		}
	}

	return nil
}

// Handler returns an HTTP handler for SSE mode.
// This should be called after the service has started.
func (s *Server) Handler() http.Handler {
	if s.cfg.Transport != TransportSSE {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "MCP SSE transport not enabled", http.StatusNotFound)
		})
	}

	if s.sseServer == nil {
		s.sseServer = server.NewSSEServer(s.mcpServer,
			server.WithStaticBasePath(s.cfg.SSEPath),
		)
	}

	return s.sseServer
}

