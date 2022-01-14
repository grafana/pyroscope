package adhoc

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
	"github.com/pyroscope-io/pyroscope/webapp"
)

type htmlWriter struct {
	enabled        bool
	maxNodesRender int
	now            time.Time
	dataDir        string
	assetsDir      http.FileSystem
	filenames      []string
}

// newHTMLWriter creates a writer of profile trees to HTML.
// The writer will store all the profiles in a temporary directory
// and once its closed it'll move the profiles to the current directory.
// If there's a single profile, the profile file is moved instead of the directory.
func newHTMLWriter(enabled bool, maxNodesRender int, now time.Time) (*htmlWriter, error) {
	var (
		dataDir   string
		assetsDir http.FileSystem
		err       error
	)

	if enabled {
		assetsDir, err = webapp.Assets()
		if err != nil {
			return nil, fmt.Errorf("could not get the asset directory: %w", err)
		}

		dataDir = fmt.Sprintf("pyroscope-adhoc-%s", now.Format("2006-01-02-15-04-05"))
		if err := os.MkdirAll(dataDir, os.ModeDir|os.ModePerm); err != nil {
			return nil, fmt.Errorf("could not create directory for HTML output: %w", err)
		}
	}

	return &htmlWriter{
		enabled:        enabled,
		maxNodesRender: maxNodesRender,
		dataDir:        dataDir,
		assetsDir:      assetsDir,
		now:            now,
	}, nil
}

func (w *htmlWriter) write(name string, out *storage.GetOutput) error {
	filename := fmt.Sprintf("%s-%s.html", name, w.now.Format("2006-01-02-15-04-05"))
	path := filepath.Join(w.dataDir, filename)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not create temporary path %s: %w", path, err)
	}
	defer f.Close()

	fb := flamebearer.NewProfile(out, w.maxNodesRender)
	if err := flamebearer.FlameberarerToHTML(&fb, w.assetsDir, f); err != nil {
		return fmt.Errorf("could not export flambearer as HTML: %w", err)
	}

	w.filenames = append(w.filenames, filename)
	return nil
}

func (w *htmlWriter) close() error {
	if !w.enabled {
		return nil
	}
	w.enabled = false
	switch len(w.filenames) {
	case 0:
		if err := os.Remove(w.dataDir); err != nil {
			return fmt.Errorf("could not remove directory %s: %w", w.dataDir, err)
		}
	case 1:
		path := filepath.Join(w.dataDir, w.filenames[0])
		if err := os.Rename(path, w.filenames[0]); err != nil {
			return fmt.Errorf("could not rename %s to %s: %w", w.filenames[0], path, err)
		}
		if err := os.Remove(w.dataDir); err != nil {
			return fmt.Errorf("could not remove directory %s: %w", w.dataDir, err)
		}
	}
	return nil
}
