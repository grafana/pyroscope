package writer

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/pyroscope-io/pyroscope/pkg/fs"
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
	return fs.EnsureDirExists(w.dataDir)
}

// Write writes a flamebearer in json format to its dataDir
func (w *AdhocDataDirWriter) Write(filename string, flame flamebearer.FlamebearerProfile) (string, error) {
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
