package testing

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/grafana/pyroscope/pkg/og/config"
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
				Database: config.Database{
					Type: "sqlite3",
				},

				Auth: config.Auth{
					SignupDefaultRole: "admin",
				},
			},
		}
	})

	ginkgo.AfterEach(func() {
		tmpDir.Close()
	})

	cb(&cfg)
}
