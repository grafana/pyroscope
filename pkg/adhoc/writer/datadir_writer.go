package writer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
)

type AdhocDataDirWriter struct {
	dataDir string
}

func NewAdhocDataDirWriter(dataDir string) *AdhocDataDirWriter {
	return &AdhocDataDirWriter{
		dataDir,
	}
}

// EnsureExist makes sure the ${dataDir} directory exists in the filesystem
func (w *AdhocDataDirWriter) EnsureExists() error {
	if err := os.MkdirAll(w.dataDir, os.ModeDir|os.ModePerm); err != nil {
		return fmt.Errorf("could not create data directory %s: %w", w.dataDir, err)
	}

	return nil
}

// Write writes a flamebearer in json format to its dataDir
// given the app name, a flamebearer and a timestamp
// TODO(eh-am): do we even need a name?
func (w *AdhocDataDirWriter) Write(filename string, flame flamebearer.FlamebearerProfile) (string, error) {
	// Remove extension
	path := filepath.Join(w.dataDir, filename)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(flame); err != nil {
		return "", err
	}

	if err := f.Close(); err != nil {
		return "", err
	}

	return path, nil
}
