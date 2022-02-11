package adhoc

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
	"github.com/pyroscope-io/pyroscope/webapp"
	"google.golang.org/protobuf/proto"
)

type externalWriter struct {
	format         string
	maxNodesRender int
	now            time.Time
	dataDir        string
	assetsDir      http.FileSystem
	filenames      []string
}

// newExternalWriter creates a writer of profile trees to external formats (see isSupported for supported formats).
// The writer will store all the profiles in a temporary directory
// and once its closed it'll move the profiles to the current directory.
// If there's a single profile, the profile file is moved instead of the directory.
func newExternalWriter(format string, maxNodesRender int, now time.Time) (*externalWriter, error) {
	var (
		dataDir   string
		assetsDir http.FileSystem
		err       error
	)

	if format == "html" {
		assetsDir, err = webapp.Assets()
		if err != nil {
			return nil, fmt.Errorf("could not get the asset directory: %w", err)
		}
	}

	if format != "none" {
		dataDir = fmt.Sprintf("pyroscope-adhoc-%s", now.Format("2006-01-02-15-04-05"))
		if err := os.MkdirAll(dataDir, os.ModeDir|os.ModePerm); err != nil {
			return nil, fmt.Errorf("could not create directory for external output: %w", err)
		}
	}

	return &externalWriter{
		format:         format,
		maxNodesRender: maxNodesRender,
		dataDir:        dataDir,
		assetsDir:      assetsDir,
		now:            now,
	}, nil
}

func (w *externalWriter) write(name string, out *storage.GetOutput) error {
	if w.format == "none" {
		return nil
	}
	var ext string
	if w.format == "collapsed" {
		ext = "collapsed.txt"
	} else {
		ext = w.format
	}

	filename := fmt.Sprintf("%s-%s.%s", name, w.now.Format("2006-01-02-15-04-05"), ext)
	path := filepath.Join(w.dataDir, filename)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not create temporary path %s: %w", path, err)
	}
	defer f.Close()

	switch w.format {
	case "pprof":
		pprof := out.Tree.Pprof(&tree.PprofMetadata{
			Unit:      out.Units,
			StartTime: w.now,
		})
		out, err := proto.Marshal(pprof)
		if err != nil {
			return fmt.Errorf("could not serialize to pprof: %w", err)
		}
		if _, err := f.Write(out); err != nil {
			return fmt.Errorf("could not write the pprof file: %w", err)
		}
	case "collapsed":
		if _, err := f.WriteString(out.Tree.Collapsed()); err != nil {
			return fmt.Errorf("could not write the collapsed file: %w", err)
		}
	case "html":
		fb := flamebearer.NewProfile(filename, out, w.maxNodesRender)
		if err := flamebearer.FlamebearerToStandaloneHTML(&fb, w.assetsDir, f); err != nil {
			return fmt.Errorf("could not write the standalone HTML file: %w", err)
		}
	}

	w.filenames = append(w.filenames, filename)
	return nil
}

func (w *externalWriter) close() (string, error) {
	if w.format == "none" {
		return "", nil
	}
	w.format = "none"
	switch len(w.filenames) {
	case 0:
		if err := os.Remove(w.dataDir); err != nil {
			return "", fmt.Errorf("could not remove directory %s: %w", w.dataDir, err)
		}
		return "", nil
	case 1:
		path := filepath.Join(w.dataDir, w.filenames[0])
		if err := os.Rename(path, w.filenames[0]); err != nil {
			return "", fmt.Errorf("could not rename %s to %s: %w", w.filenames[0], path, err)
		}
		if err := os.Remove(w.dataDir); err != nil {
			return "", fmt.Errorf("could not remove directory %s: %w", w.dataDir, err)
		}
		return w.filenames[0], nil
	}
	return w.dataDir, nil
}
