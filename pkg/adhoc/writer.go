package adhoc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
)

type writer struct {
	maxNodesRender int
	outputFormat   string
	logger         *logrus.Logger
	storage        *storage.Storage
}

func newWriter(cfg *config.Adhoc, st *storage.Storage, logger *logrus.Logger) writer {
	return writer{
		maxNodesRender: cfg.MaxNodesRender,
		outputFormat:   cfg.OutputFormat,
		logger:         logger,
		storage:        st,
	}
}

func (w writer) write(t0, t1 time.Time) error {
	dataDir := dataDirectory()
	if err := os.MkdirAll(dataDir, os.ModeDir|os.ModePerm); err != nil {
		return fmt.Errorf("could not create data directory: %w", err)
	}

	for _, name := range w.storage.GetAppNames() {
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
		out, err := w.storage.Get(gi)
		if err != nil {
			w.logger.WithError(err).Error("retrieving storage key")
			continue
		}
		if out == nil {
			w.logger.Warn("no data retrieved")
			continue
		}

		var ext string
		if w.outputFormat == "collapsed" {
			ext = "collapsed.txt"
		} else {
			ext = w.outputFormat
		}
		filename := fmt.Sprintf("%s-%s.%s", name, t0.UTC().Format(time.RFC3339), ext)
		path := filepath.Join(dataDir, filename)
		f, err := os.Create(path)
		if err != nil {
			w.logger.WithError(err).Error("creating output file")
			continue
		}
		defer f.Close()
		switch w.outputFormat {
		case "json":
			res := flamebearer.NewProfile(out, w.maxNodesRender)
			if err := json.NewEncoder(f).Encode(res); err != nil {
				w.logger.WithError(err).Error("saving output file")
			}
		case "pprof":
			pprof := out.Tree.Pprof(&tree.PprofMetadata{
				Unit:      out.Units,
				StartTime: t0,
			})
			out, err := proto.Marshal(pprof)
			if err != nil {
				w.logger.WithError(err).Error("serializing to pprof")
			}
			if _, err := f.Write(out); err != nil {
				w.logger.WithError(err).Error("saving output file")
			}
		case "collapsed":
			if _, err := f.WriteString(out.Tree.Collapsed()); err != nil {
				w.logger.WithError(err).Error("saving output file")
			}
		}
		w.logger.Infof("profiling data has been saved to %s", path)
		if err := f.Close(); err != nil {
			w.logger.WithError(err).Error("closing output file")
		}
	}
	return nil
}

func dataDirectory() string {
	return filepath.Join(dataBaseDirectory(), "pyroscope")
}
