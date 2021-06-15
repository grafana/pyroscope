// +build !windows

package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func startServer(c *config.Server) error {
	logLevel, err := logrus.ParseLevel(c.LogLevel)
	if err != nil {
		return err
	}
	logrus.SetLevel(logLevel)
	srv, err := newServerService(logrus.StandardLogger(), c)
	if err != nil {
		return fmt.Errorf("could not initialize server: %w", err)
	}

	go func() {
		err = srv.Start()
	}()

	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM)
	<-s

	srv.Stop()
	return err
}
