package debug

import (
	"os"
	"path/filepath"
	"regexp"

	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

var nameRegexp = regexp.MustCompile("[^a-zA-Z0-9_]+")

func DiskUsage(path string) map[string]interface{} {
	f := map[string]interface{}{}
	subdirectories, _ := filepath.Glob(filepath.Join(path, "*"))
	for _, path := range subdirectories {
		name := filepath.Base(path)
		name = nameRegexp.ReplaceAllString(name, "_")
		f[name] = dirSize(path)
	}

	return f
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
	return result
}
