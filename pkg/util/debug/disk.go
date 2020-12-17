package debug

import (
	"os"
	"path/filepath"

	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
	"github.com/sirupsen/logrus"
)

func PrintDiskUsage(path string) {
	f := logrus.Fields{}
	subdirectories, _ := filepath.Glob(filepath.Join(path, "*"))
	for _, path := range subdirectories {
		f[filepath.Base(path)] = dirSize(path).String()
	}

	logrus.WithFields(f).Debug("Disk stats")
}

func dirSize(path string) (result bytesize.ByteSize) {
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			result += bytesize.ByteSize(info.Size())
		}
		return nil
	})
	return
}
