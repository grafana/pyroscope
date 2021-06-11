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
		cfg = &config.Config{
			Server: config.Server{
				StoragePath: tmpDir.Path,
				APIBindAddr: ":4040",

				CacheSegmentSize:    100,
				CacheTreeSize:       100,
				CacheDictionarySize: 100,
				CacheDimensionSize:  100,
				CacheEvictThreshold: 0.02,
				CacheEvictVolume:    0.10,

				MaxNodesSerialization: 2048,
				MaxNodesRender:        2048,

				OutOfSpaceThreshold: 512 * 1024 * 1024, // bytes (default: 512MB)
			},
		}
	})

	ginkgo.AfterEach(func() {
		tmpDir.Close()
	})

	cb(&cfg)
}
