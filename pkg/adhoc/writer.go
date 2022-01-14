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
	outputHTML     bool
	logger         *logrus.Logger
	storage        *storage.Storage
	dataDir        string
}

func newWriter(cfg *config.Adhoc, st *storage.Storage, logger *logrus.Logger) writer {
	return writer{
		maxNodesRender: cfg.MaxNodesRender,
		outputFormat:   cfg.OutputFormat,
		outputHTML:     !cfg.NoStandaloneHTML,
		logger:         logger,
		storage:        st,
		dataDir:        cfg.DataPath,
	}
}

func (w writer) write(t0, t1 time.Time) error {
	if err := os.MkdirAll(w.dataDir, os.ModeDir|os.ModePerm); err != nil {
		return fmt.Errorf("could not create data directory: %w", err)
	}
	hw, err := newHTMLWriter(w.outputHTML, w.maxNodesRender, t0)
	if err != nil {
		return fmt.Errorf("could not create the HTML writer: %w", err)
	}
	defer hw.close() // It's fine to call this multiple times

	profiles := 0
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
		filename := fmt.Sprintf("%s-%s.%s", name, t0.Format("2006-01-02-15-04-05"), ext)
		path := filepath.Join(w.dataDir, filename)
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
		if err := hw.write(name, out); err != nil {
			w.logger.WithError(err).Error("saving HTML file")
		}
		w.logger.Infof("profiling data has been saved to %s", path)
		profiles++
		if err := f.Close(); err != nil {
			w.logger.WithError(err).Error("closing output file")
		}
	}
	if err := hw.close(); err != nil {
		w.logger.WithError(err).Error("closing HTML writer")
	}
	if profiles == 0 {
		w.logger.Warning("no profiling data was saved, maybe the profiled process didn't run long enough?")
	} else {
		switch len(hw.filenames) {
		case 0:
			w.logger.Info("you can now run `pyroscope server` and see the profiling data at http://localhost:4040/adhoc-single")
		case 1:
			w.logger.Infof(
				"you can now open the HTML file '%s' or run `pyroscope server` and see the profiling data at http://localhost:4040/adhoc-single",
				hw.filenames[0],
			)
		default:
			w.logger.Infof(
				"you can now open the HTML files in 'pyroscope-adhoc-%s' or run `pyroscope server` and see the profiling data at http://localhost:4040/adhoc-single",
				t0.Format("2006-01-02-15-04-05"),
			)
		}
	}
	return nil
}
