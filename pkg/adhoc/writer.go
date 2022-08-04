package adhoc

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	adhocWriter "github.com/pyroscope-io/pyroscope/pkg/adhoc/writer"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
)

type writer struct {
	maxNodesRender int
	outputFormat   string
	outputJSON     bool
	logger         *logrus.Logger
	storage        *storage.Storage

	adhocDataDirWriter *adhocWriter.AdhocDataDirWriter
}

func newWriter(cfg *config.Adhoc, st *storage.Storage, logger *logrus.Logger) writer {
	return writer{
		maxNodesRender:     cfg.MaxNodesRender,
		outputFormat:       cfg.OutputFormat,
		outputJSON:         !cfg.NoJSONOutput,
		logger:             logger,
		storage:            st,
		adhocDataDirWriter: adhocWriter.NewAdhocDataDirWriter(cfg.DataPath),
	}
}

func (w writer) write(t0, t1 time.Time) error {
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

		if err := ew.write(name, out); err != nil {
			w.logger.WithError(err).Error("saving output file")
			continue
		}

		if w.outputJSON {
			flame := flamebearer.NewProfile(name, out, w.maxNodesRender)
			filename := fmt.Sprintf("%s-%s.json", name, t0.Format("2006-01-02-15-04-05"))
			err = w.adhocDataDirWriter.Write(filename, flame)

			if err != nil {
				w.logger.WithError(err).Error("saving to AdhocDir")
				continue
			}
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
