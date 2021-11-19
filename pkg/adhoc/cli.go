package adhoc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exec"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
)

func Cli(cfg *config.Adhoc, args []string) error {
	logLevel, err := logrus.ParseLevel(cfg.Server.LogLevel)
	if err != nil {
		return fmt.Errorf("could not parse log level: %w", err)
	}
	logrus.SetLevel(logLevel)
	logger := logrus.StandardLogger()

	switch cfg.OutputFormat {
	case "json", "pprof", "collapsed":
	default:
		return fmt.Errorf("invalid output format '%s', the only supported output formats are 'json', 'pprof' and 'collapsed'", cfg.OutputFormat)
	}

	st, err := storage.New(storage.NewConfig(cfg.Server).WithInMemory(), logger, prometheus.DefaultRegisterer)
	if err != nil {
		return fmt.Errorf("could not initialize storage: %w", err)
	}

	g, ctx := errgroup.WithContext(context.Background())
	// TODO(abeaumont): Server shouldn't have access to the storage and only be run depending on the config options.
	g.Go(func() error {
		return cli.StartAdhocServer(ctx, cfg.Server, st, logger)
	})
	g.Go(func() error {
		t0 := time.Now()
		if err := exec.Cli(exec.NewConfig(cfg.Exec).WithAdhoc(), args, st, logger); err != nil {
			return err
		}
		t1 := time.Now()
		dataDir := dataDirectory()
		if err := os.MkdirAll(dataDir, os.ModeDir|os.ModePerm); err != nil {
			return fmt.Errorf("could not create data directory: %w", err)
		}

		for _, name := range st.GetAppNames() {
			skey, err := segment.ParseKey(name)
			if err != nil {
				logger.WithError(err).Error("parsing storage key")
				continue
			}
			gi := &storage.GetInput{
				StartTime: t0,
				EndTime:   t1,
				Key:       skey,
			}
			out, err := st.Get(gi)
			if err != nil {
				logger.WithError(err).Error("retrieving storage key")
				continue
			}
			if out == nil {
				logger.Warn("no data retrieved")
				continue
			}

			var ext string
			if cfg.OutputFormat == "collapsed" {
				ext = "collapsed.txt"
			} else {
				ext = cfg.OutputFormat
			}
			filename := fmt.Sprintf("%s-%s.%s", name, t0.UTC().Format(time.RFC3339), ext)
			path := filepath.Join(dataDir, filename)
			f, err := os.Create(path)
			if err != nil {
				logger.WithError(err).Error("creating output file")
				continue
			}
			defer f.Close()
			switch cfg.OutputFormat {
			case "json":
				// TODO(abeaumont): This is duplicated code, fix the original first.
				fs := out.Tree.FlamebearerStruct(cfg.Server.MaxNodesRender)
				fs.SpyName = out.SpyName
				fs.SampleRate = out.SampleRate
				fs.Units = out.Units
				res := map[string]interface{}{
					"timeline":    out.Timeline,
					"flamebearer": fs,
					"metadata": map[string]interface{}{
						"format":     fs.Format, // "single" | "double"
						"spyName":    out.SpyName,
						"sampleRate": out.SampleRate,
						"units":      out.Units,
					},
				}
				if err := json.NewEncoder(f).Encode(res); err != nil {
					logger.WithError(err).Error("saving output file")
				}
			case "pprof":
				pprof := out.Tree.Pprof(&tree.PprofMetadata{
					Unit:      out.Units,
					StartTime: t0,
				})
				out, err := proto.Marshal(pprof)
				if err != nil {
					logger.WithError(err).Error("serializing to pprof")
				}
				if _, err := f.Write(out); err != nil {
					logger.WithError(err).Error("saving output file")
				}
			case "collapsed":
				if _, err := f.WriteString(out.Tree.Collapsed()); err != nil {
					logger.WithError(err).Error("saving output file")
				}
			}
			logger.Debugf("exported data to %s", path)
			if err := f.Close(); err != nil {
				logger.WithError(err).Error("closing output file")
			}
		}
		return nil
	})
	err = g.Wait()
	logger.Debug("stopping storage")
	if err := st.Close(); err != nil {
		logger.WithError(err).Error("storage close")
	}
	return err
}

func dataDirectory() string {
	return filepath.Join(dataBaseDirectory(), "pyroscope")
}
