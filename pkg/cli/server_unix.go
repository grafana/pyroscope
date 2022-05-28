//go:build !windows
// +build !windows

package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func NewServer(c *config.Server) (*Server, error) {
	svc, err := newServerService(c)
	if err != nil {
		return nil, fmt.Errorf("could not initialize server: %w", err)
	}
	return &Server{svc: svc}, nil
}

func (s *Server) Stop() { s.svc.Stop() }

func (s *Server) Start() error {
	exited := make(chan error)
	go func() {
		exited <- s.svc.Start()
		close(exited)
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	for {
		select {
		case err := <-exited:
			return err

		case <-sigs:
			s.svc.logger.Info("stopping server")
			stopTime := time.Now()
			s.svc.Stop()
			if err := <-exited; err != nil {
				s.svc.logger.WithError(err).Error("failed to stop server gracefully")
				return err
			}
			s.svc.logger.WithField("duration", time.Since(stopTime)).Info("server stopped gracefully")
			return nil
		}
	}
}
