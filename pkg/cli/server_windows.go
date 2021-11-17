package cli

import (
	"fmt"
)

func NewServer(_ ConfigProvider) (*Server, error) {
	return nil, fmt.Errorf("server mode is not supported on Windows")
}

func (s *Server) Stop() { return }

func (s *Server) Start() error { return nil }
