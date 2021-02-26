package testing

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"

	"github.com/onsi/ginkgo"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

func DirStats(path string) (directories int, files int, size bytesize.ByteSize) {
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
	return
}

func TmpDir(cb func(name string)) {
	defer ginkgo.GinkgoRecover()
	path, err := ioutil.TempDir("/tmp", "pyroscope-test-dir")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(path)

	logrus.Debug("tmpDir:", path)
	cb(path)
	// return dirSize(path)
}

type TmpDirectory struct {
	Path string
}

func (t *TmpDirectory) Close() {
	os.RemoveAll(t.Path)
}

func TmpDirSync() *TmpDirectory {
	defer ginkgo.GinkgoRecover()
	path, err := ioutil.TempDir("/tmp", "pyroscope-test-dir")
	if err != nil {
		panic(err)
	}
	return &TmpDirectory{Path: path}
}
