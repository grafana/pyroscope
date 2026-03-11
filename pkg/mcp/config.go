package mcp

import (
	"flag"
	"fmt"
)

const (
	TransportStdio = "stdio"
	TransportSSE   = "sse"
)

// Config holds the configuration for the MCP server.
type Config struct {
	Enabled   bool   `yaml:"enabled"`
	Transport string `yaml:"transport"`
	SSEPath   string `yaml:"sse_path"`
}

// RegisterFlags registers MCP-related flags.
func (c *Config) RegisterFlags(f *flag.FlagSet) {
	f.BoolVar(&c.Enabled, "mcp.enabled", false, "Enable MCP (Model Context Protocol) server for AI assistant integration.")
	f.StringVar(&c.Transport, "mcp.transport", TransportSSE, "MCP transport mode: 'stdio' or 'sse'. In stdio mode, MCP hijacks stdin/stdout.")
	f.StringVar(&c.SSEPath, "mcp.sse-path", "/mcp", "HTTP path for MCP SSE endpoint (only used when transport is 'sse').")
}

// Validate validates the MCP configuration.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.Transport != TransportStdio && c.Transport != TransportSSE {
		return fmt.Errorf("invalid MCP transport %q: must be %q or %q", c.Transport, TransportStdio, TransportSSE)
	}

	if c.Transport == TransportSSE && c.SSEPath == "" {
		return fmt.Errorf("MCP SSE path cannot be empty when transport is %q", TransportSSE)
	}

	return nil
}

