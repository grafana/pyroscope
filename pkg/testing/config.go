package testing

import (
	"github.com/onsi/ginkgo"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"time"
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

				CacheSegmentSize:    10,
				CacheTreeSize:       10,
				CacheDictionarySize: 10,
				CacheDimensionSize:  10,

				MaxNodesSerialization: 2048,
				MaxNodesRender:        2048,

				OutOfSpaceThreshold: 512 * 1024 * 1024, // bytes (default: 512MB)
				ThresholdModeAuto:   true,
				RetentionThreshold:  time.Hour * 24 * 7, // (default: 3days)
			},
		}
	})

	ginkgo.AfterEach(func() {
		tmpDir.Close()
	})

	cb(&cfg)
}
