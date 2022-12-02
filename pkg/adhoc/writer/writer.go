package writer

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
)

type Writer struct {
	maxNodesRender int
	outputFormat   string
	outputJSON     bool
	logger         *logrus.Logger
	storage        *storage.Storage

	adhocDataDirWriter *AdhocDataDirWriter
	stripTimestamp     bool
}

func NewWriter(cfg *config.Adhoc, st *storage.Storage, logger *logrus.Logger) Writer {
	return Writer{
		maxNodesRender:     cfg.MaxNodesRender,
		outputFormat:       cfg.OutputFormat,
		outputJSON:         !cfg.NoJSONOutput,
		logger:             logger,
		storage:            st,
		adhocDataDirWriter: NewAdhocDataDirWriter(cfg.DataPath),
		stripTimestamp:     cfg.StripTimestamp,
	}
}

func (w Writer) Write(t0, t1 time.Time) error {
	err := w.adhocDataDirWriter.EnsureExists()
	if err != nil {
		return err
	}
	ew, err := newExternalWriter(w.outputFormat, w.maxNodesRender, t0)
	if err != nil {
		return fmt.Errorf("could not create the external writer: %w", err)
	}
	defer ew.close() // It's fine to call this multiple times

	profiles := 0

	// The assumption is that these were the only ingested apps
	for _, name := range w.storage.GetAppNames(context.TODO()) {
		skey, err := segment.ParseKey(name)
		if err != nil {
			w.logger.WithError(err).Error("parsing storage key")
			continue
		}
		gi := &storage.GetInput{
			StartTime: t0,
			EndTime:   t1,
			Key:       skey,
		}
		out, err := w.storage.Get(context.TODO(), gi)

		if err != nil {
			w.logger.WithError(err).Error("retrieving storage key")
			continue
		}
		if out == nil {
			w.logger.Warn("no data retrieved")
			continue
		}

		// TODO: Remove stripTimestamp flag and instead receive a formatter
		if err := ew.write(name, out, w.stripTimestamp); err != nil {
			w.logger.WithError(err).Error("saving output file")
			continue
		}

		if w.outputJSON {
			flame := flamebearer.NewProfile(flamebearer.ProfileConfig{
				Name:      name,
				MaxNodes:  w.maxNodesRender,
				Tree:      out.Tree,
				Timeline:  out.Timeline,
				Groups:    out.Groups,
				Telemetry: out.Telemetry,
				Metadata: metadata.Metadata{
					SpyName:         out.SpyName,
					SampleRate:      out.SampleRate,
					Units:           out.Units,
					AggregationType: out.AggregationType,
				},
			})
			filename := fmt.Sprintf("%s-%s.json", name, t0.Format("2006-01-02-15-04-05"))
			path, err := w.adhocDataDirWriter.Write(filename, flame)
			if err != nil {
				w.logger.WithError(err).Error("saving to AdhocDir")
				continue
			}

			w.logger.Infof("profiling data has been saved to %s", path)
		}

		profiles++
	}

	path, err := ew.close()
	if err != nil {
		w.logger.WithError(err).Error("closing external writer")
	}
	if path == "" {
		if profiles == 0 {
			w.logger.Warning("no profiling data was saved, maybe the profiled process didn't run long enough?")
		} else {
			w.logger.Info("you can now run `pyroscope server` and see the profiling data at http://localhost:4040/adhoc-single")
		}
	} else {
		if profiles == 0 {
			w.logger.Infof("profiling data was saved in '%s'", path)
		} else {
			w.logger.Infof(
				"profiling data was saved in '%s' and you can also run `pyroscope server` to see the profiling data at http://localhost:4040/adhoc-single",
				path,
			)
		}
	}
	return nil
}
