package config

import (
	"time"
)

type Config struct {
	Version bool

	Agent     Agent     `skip:"true"`
	Server    Server    `skip:"true"`
	Convert   Convert   `skip:"true"`
	Exec      Exec      `skip:"true"`
	DbManager DbManager `skip:"true"`
}

type Agent struct {
	Config   string `def:"<installPrefix>/etc/pyroscope/agent.yml" desc:"location of config file"`
	LogLevel string `def:"info", desc:"debug|info|warn|error"`

	// AgentCMD           []string
	AgentSpyName           string        `desc:"name of the spy you want to use"` // TODO: add options
	AgentPID               int           `def:"-1" desc:"pid of the process you want to spy on"`
	ServerAddress          string        `def:"http://localhost:4040" desc:"address of the pyroscope server"`
	AuthToken              string        `def:"" desc:"authorization token used to upload profiling data"`
	UpstreamThreads        int           `def:"4"`
	UpstreamRequestTimeout time.Duration `def:"10s"`
	UNIXSocketPath         string        `def:"<installPrefix>/var/run/pyroscope-agent.sock" desc:"path to a UNIX socket file"`
}

type Server struct {
	Config         string `def:"<installPrefix>/etc/pyroscope/server.yml" desc:"location of config file"`
	LogLevel       string `def:"info", desc:"debug|info|warn|error"`
	BadgerLogLevel string `def:"error", desc:"debug|info|warn|error"`

	StoragePath string `def:"<installPrefix>/var/lib/pyroscope" desc:"directory where pyroscope stores profiling data"`
	ApiBindAddr string `def:":4040" desc:"port for the HTTP server used for data ingestion and web UI"`

	// These will eventually be replaced by some sort of a system that keeps track of RAM
	//   and updates
	CacheDimensionSize  int `def:"1000" desc:"max number of elements in LRU cache for dimensions"`
	CacheDictionarySize int `def:"1000" desc:"max number of elements in LRU cache for dictionaries"`
	CacheSegmentSize    int `def:"1000" desc:"max number of elements in LRU cache for segments"`
	CacheTreeSize       int `def:"10000" desc:"max number of elements in LRU cache for trees"`

	// TODO: I don't think a lot of people will change these values.
	//   I think these should just be constants.
	Multiplier      int           `skip:"true" def:"10"`
	MinResolution   time.Duration `skip:"true" def:"10s"`
	MaxResolution   time.Duration `skip:"true" def:"8760h"` // 365 days
	StorageMaxDepth int           `skip:"true"`

	MaxNodesSerialization int `def:"2048" desc:"max number of nodes used when saving profiles to disk"`
	MaxNodesRender        int `def:"2048" desc:"max number of nodes used to display data on the frontend"`

	// currently only used in our demo app
	HideApplications []string `def:""`

	AnalyticsOptOut bool `def:"false" desc:"disables analytics"`
}

type Convert struct {
	Format string `def:"tree"`
}

type DbManager struct {
	LogLevel        string `def:"error", desc:"debug|info|warn|error"`
	StoragePath     string `def:"<installPrefix>/var/lib/pyroscope" desc:"directory where pyroscope stores profiling data"`
	DstStartTime    time.Time
	DstEndTime      time.Time
	SrcStartTime    time.Time
	ApplicationName string

	EnableProfiling bool `def:"false" desc:"enables profiling of dbmanager"`
}

type Exec struct {
	SpyName                string        `def:"auto" desc:"name of the profiler you want to use. Supported ones are: <supportedProfilers>"`
	ApplicationName        string        `def:"" desc:"application name used when uploading profiling data"`
	DetectSubprocesses     bool          `def:"true" desc:"makes pyroscope keep track of and profile subprocesses of the main process"`
	LogLevel               string        `def:"info", desc:"debug|info|warn|error"`
	ServerAddress          string        `def:"http://localhost:4040" desc:"address of the pyroscope server"`
	AuthToken              string        `def:"" desc:"authorization token used to upload profiling data"`
	UpstreamThreads        int           `def:"4" desc:"number of upload threads"`
	UpstreamRequestTimeout time.Duration `def:"10s" desc:"profile upload timeout"`
	NoLogging              bool          `def:"false" desc:"disables logging from pyroscope"`
	NoRootDrop             bool          `def:"false" desc:"disables permissions drop when ran under root. use this one if you want to run your command as root"`
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
			ApiBindAddr: ":4040",

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
