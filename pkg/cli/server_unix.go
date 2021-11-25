// +build !windows

package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func StartServer(c *config.Server) error {
	logLevel, err := logrus.ParseLevel(c.LogLevel)
	if err != nil {
		return err
	}
	logrus.SetLevel(logLevel)
	logger := logrus.StandardLogger()
	srv, err := newServerService(logger, c)
	if err != nil {
		return fmt.Errorf("could not initialize server: %w", err)
	}

	if srv.config.Auth.JWTSecret == "" {
		srv.config.Auth.JWTSecret, err = srv.storage.JWT()
		if err != nil {
			return err
		}
	}

	var stopTime time.Time
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM)

	exited := make(chan error)
	go func() {
		exited <- srv.Start()
		close(exited)
	}()

	select {
	case <-s:
		logger.Info("stopping server")
		stopTime = time.Now()
		srv.Stop()
		if err = <-exited; err != nil {
			logger.WithError(err).Error("failed to stop server gracefully")
			return err
		}
		logger.WithField("duration", time.Since(stopTime)).Info("server stopped gracefully")
		return nil

	case err = <-exited:
		if err == nil {
			// Should never happen.
			logger.Error("server exited")
			return nil
		}
		logger.WithError(err).Error("server failed")
		return err
	}
}
