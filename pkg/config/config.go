package config

import (
	"time"

	"github.com/peterbourgon/ff/v3/ffcli"
)

type Agent struct {
	Config string `def:"/etc/pyroscope/agent.yml" desc:"location of config file"`

	// AgentCMD           []string
	AgentSpyName           string        `desc:"name of the spy you want to use"` // TODO: add options
	AgentPID               int           `def:"-1" desc:"pid of the process you want to spy on"`
	AgentControlSocket     string        `def:"/var/run/pyroscope/agent.sock" desc:"path to a UNIX socket file"`
	UpstreamAddress        string        `def:"http://localhost:8080" desc:"address of the pyroscope server"`
	UpstreamThreads        int           `def:"4"`
	UpstreamRequestTimeout time.Duration `def:"10s"`
}
type Server struct {
	Config string `def:"/etc/pyroscope/server.yml" desc:"location of config file"`

	StoragePath string `def:"tmp/pyroscope-storage"`
	ApiBindAddr string `def:":8080"`

	CacheSegmentSize int `def:"1000"`
	CacheTreeSize    int `def:"1000"`

	Multiplier      int           `def:"10"`
	MinResolution   time.Duration `def:"10s"`
	MaxResolution   time.Duration `def:"8760h"` // 365 days
	StorageMaxDepth int           `def:"10"`

	MaxNodesSerialization int `def:"1024"`
	MaxNodesSVG           int `def:"1024"`
}
type Config struct {
	ffCommand  *ffcli.Command `skip:"true"`
	Subcommand string         `skip:"true"`
	Version    bool

	Agent  Agent  `skip:"true"`
	Server Server `skip:"true"`
}

func calculateMaxDepth(min, max time.Duration, multiplier int) (depth int) {
	for min < max {
		min *= time.Duration(multiplier)
		depth++
	}
	return
}

func New() *Config {
	cfg := &Config{
		Server: Server{
			StoragePath: "tmp/pyroscope-storage",
			ApiBindAddr: ":8080",

			CacheSegmentSize: 1000,
			CacheTreeSize:    1000,

			Multiplier:    10,
			MinResolution: 10 * time.Second,
			MaxResolution: time.Hour * 24 * 365 * 5,
		},
	}

	cfg.Server.StorageMaxDepth = calculateMaxDepth(cfg.Server.MinResolution, cfg.Server.MaxResolution, cfg.Server.Multiplier)

	// flag.Parse()

	return cfg
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
