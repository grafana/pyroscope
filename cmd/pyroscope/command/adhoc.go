package command

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exec"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

func newAdhocCmd(cfg *config.Adhoc) *cobra.Command {
	vpr := newViper()
	adhocCmd := &cobra.Command{
		Use:   "adhoc [flags]",
		Short: "Start a new process from arguments, profile it and view the results",
		Args:  cobra.MinimumNArgs(1),

		DisableFlagParsing: true,
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, args []string) error {
			logLevel, err := logrus.ParseLevel(cfg.Server.LogLevel)
			if err != nil {
				return fmt.Errorf("could not parse log level: %w", err)
			}
			logrus.SetLevel(logLevel)
			logger := logrus.StandardLogger()

			storage, err := storage.New(storage.NewConfig(cfg.Server).WithInMemory(), logger, prometheus.DefaultRegisterer)
			if err != nil {
				return fmt.Errorf("could not initialize storage: %w", err)
			}

			g, ctx := errgroup.WithContext(context.Background())
			g.Go(func() error {
				return cli.StartAdhocServer(ctx, cfg.Server, storage, logger)
			})
			g.Go(func() error {
				return exec.Cli(exec.NewConfig(cfg.Exec).WithAdhoc(), args, storage, logger)
			})
			err = g.Wait()
			logger.Debug("stopping storage")
			if err := storage.Close(); err != nil {
				logger.WithError(err).Error("storage close")
			}
			return err
		}),
	}

	cli.PopulateFlagSet(cfg.Exec, adhocCmd.Flags(), vpr, cli.WithSkip(
		"auth-token",
		"log-level",
		"pid",
		"server-address",
		"upstream-threads",
		"upstream-request-timeout",
		"tags",
	))
	cli.PopulateFlagSet(cfg.Server, adhocCmd.Flags(), vpr, cli.WithSkip(
		"badger-no-truncate",
		"cache-evict-threshold",
		"cache-evict-volume",
		"cache-dimensions-size",
		"cache-dictonary-size",
		"cache-segment-size",
		"cache-tree-size",
		"hide-applications",
		"max-nodes-serialization",
		"metrics-export-rules",
		"out-of-space-threshold",
		"retention",
		"retention-levels",
		"sample-rate",
		"storage-path",
	))
	return adhocCmd
}
