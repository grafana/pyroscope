package agent

import "github.com/petethepig/pyroscope/pkg/config"

func Main(cfg *config.Config) {
	a := New(cfg)
	a.Start()
}
