package config

import (
	"time"
)

type Config struct {
	Version bool

	Agent  Agent  `skip:"true"`
	Server Server `skip:"true"`
}

type Agent struct {
	Config string `def:"/etc/pyroscope/agent.yml" desc:"location of config file"`

	// AgentCMD           []string
	AgentSpyName           string        `desc:"name of the spy you want to use"` // TODO: add options
	AgentPID               int           `def:"-1" desc:"pid of the process you want to spy on"`
	AgentControlSocket     string        `def:"/var/run/pyroscope/agent.sock" desc:"path to a UNIX socket file"`
	UpstreamAddress        string        `def:"http://localhost:8080" desc:"address of the pyroscope server"`
	UpstreamThreads        int           `def:"4"`
	UpstreamRequestTimeout time.Duration `def:"10s"`
	UNIXSocketPath         string        `def:"/tmp/pyroscope-socket"`
}

type Server struct {
	Config string `def:"/etc/pyroscope/server.yml" desc:"location of config file"`

	StoragePath string `def:"tmp/pyroscope-storage"`
	ApiBindAddr string `def:":8080"`

	CacheDimensionSize  int `def:"1000"`
	CacheDictionarySize int `def:"1000"`
	CacheSegmentSize    int `def:"1000"`
	CacheTreeSize       int `def:"1000"`

	Multiplier      int           `def:"10"`
	MinResolution   time.Duration `def:"10s"`
	MaxResolution   time.Duration `def:"8760h"` // 365 days
	StorageMaxDepth int           `skip:"true"`

	MaxNodesSerialization int `def:"8192"`
	MaxNodesSVG           int `def:"2048"`
}

func calculateMaxDepth(min, max time.Duration, multiplier int) int {
	depth := 0
	for min < max {
		min *= time.Duration(multiplier)
		depth++
	}
	return depth
}

// TODO: remove these preset configs
func New() *Config {
	return NewForTests("tmp/pyroscope-storage")
}

func NewForTests(path string) *Config {
	cfg := &Config{
		Server: Server{
			StoragePath: path,
			ApiBindAddr: ":8080",

			CacheSegmentSize: 10,
			CacheTreeSize:    10,

			Multiplier:    10,
			MinResolution: 10 * time.Second,
			MaxResolution: time.Hour * 24 * 365 * 5,
		},
	}

	cfg.Server.StorageMaxDepth = calculateMaxDepth(cfg.Server.MinResolution, cfg.Server.MaxResolution, cfg.Server.Multiplier)

	return cfg
}
