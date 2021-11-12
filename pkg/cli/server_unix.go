// +build !windows

package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

func StartServer(ctx context.Context, c *config.Server) error {
	logLevel, err := logrus.ParseLevel(c.LogLevel)
	if err != nil {
		return err
	}
	logrus.SetLevel(logLevel)
	logger := logrus.StandardLogger()

	st, err := storage.New(storage.NewConfig(c), logger, prometheus.DefaultRegisterer)
	if err != nil {
		return fmt.Errorf("new storage: %w", err)
	}

	srv, err := newServerService(st, logger, c, false)
	if err != nil {
		return fmt.Errorf("could not initialize server: %w", err)
	}

	if srv.config.Auth.JWTSecret == "" {
		srv.config.Auth.JWTSecret, err = srv.storage.JWT()
		if err != nil {
			return err
		}
	}

	err = run(ctx, srv, logger)
	logger.Debug("stopping storage")
	if err := st.Close(); err != nil {
		logger.WithError(err).Error("storage close")
	}
	return err
}

func StartAdhocServer(ctx context.Context, c *config.Server, st *storage.Storage, logger *logrus.Logger) error {
	srv, err := newServerService(st, logger, c, true)
	if err != nil {
		return fmt.Errorf("could not initialize server: %w", err)
	}

	return run(ctx, srv, logger)
}

func run(ctx context.Context, srv *serverService, logger *logrus.Logger) error {
	var stopTime time.Time
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM)

	exited := make(chan error)
	go func() {
		exited <- srv.Start(ctx)
		close(exited)
	}()

	select {
	case <-s:
		logger.Info("stopping server")
		stopTime = time.Now()
		srv.Stop()
		if err := <-exited; err != nil {
			logger.WithError(err).Error("failed to stop server gracefully")
			return err
		}
		logger.WithField("duration", time.Since(stopTime)).Info("server stopped gracefully")
		return nil

	case err := <-exited:
		if err == nil {
			// Should never happen.
			logger.Error("server exited")
			return nil
		}
		logger.WithError(err).Error("server failed")
		return err
	}
}
