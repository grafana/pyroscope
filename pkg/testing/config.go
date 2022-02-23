package testing

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func WithConfig(cb func(cfg **config.Config)) {
	var tmpDir *TmpDirectory
	var cfg *config.Config

	ginkgo.BeforeEach(func() {
		tmpDir = TmpDirSync()
		cfg = &config.Config{
			Server: config.Server{
				StoragePath: tmpDir.Path,
				APIBindAddr: ":4040",

				CacheEvictThreshold: 0.02,
				CacheEvictVolume:    0.10,

				MaxNodesSerialization: 2048,
				MaxNodesRender:        2048,
			},
		}
	})

	ginkgo.AfterEach(func() {
		tmpDir.Close()
	})

	cb(&cfg)
}
