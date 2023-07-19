package testing

import (
	"os"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	"github.com/grafana/pyroscope/pkg/og/util/bytesize"
)

func DirStats(path string) (directories, files int, size bytesize.ByteSize) {
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			directories++
		} else {
			files++
			size += bytesize.ByteSize(info.Size())
		}
		return nil
	})
	if err != nil {
		return -1, -1, -1
	}

	return directories, files, size
}

func TmpDir(cb func(name string)) {
	defer ginkgo.GinkgoRecover()
	path, err := os.MkdirTemp("", "pyroscope-test-dir")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(path)

	cb(path)
}

type TmpDirectory struct {
	Path string
}

func (t *TmpDirectory) Close() {
	os.RemoveAll(t.Path)
}

func TmpDirSync() *TmpDirectory {
	defer ginkgo.GinkgoRecover()
	path, err := os.MkdirTemp("", "pyroscope-test-dir")
	if err != nil {
		panic(err)
	}
	return &TmpDirectory{Path: path}
}
