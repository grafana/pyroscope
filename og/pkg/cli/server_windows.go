package cli

import (
	"fmt"

	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func NewServer(_ *config.Server) (*Server, error) {
	return nil, fmt.Errorf("server mode is not supported on Windows")
}

func (s *Server) Stop() { return }

func (s *Server) Start() error { return nil }
