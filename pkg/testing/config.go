package testing

import (
	"github.com/onsi/ginkgo"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func WithConfig(cb func(cfg **config.Config)) {
	var tmpDir *TmpDirectory
	var cfg *config.Config

	ginkgo.BeforeEach(func() {
		tmpDir = TmpDirSync()
		cfg = config.NewForTests(tmpDir.Path)
	})

	ginkgo.AfterEach(func() {
		tmpDir.Close()
	})

	cb(&cfg)
}
