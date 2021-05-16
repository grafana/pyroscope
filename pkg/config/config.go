package config

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
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
	LogLevel string `def:"info" desc:"log level: debug|info|warn|error"`

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
	AnalyticsOptOut bool `def:"false" desc:"disables analytics"`

	Config         string `def:"<installPrefix>/etc/pyroscope/server.yml" desc:"location of config file"`
	LogLevel       string `def:"info" desc:"log level: debug|info|warn|error"`
	BadgerLogLevel string `def:"error" desc:"log level: debug|info|warn|error"`

	StoragePath string `def:"<installPrefix>/var/lib/pyroscope" desc:"directory where pyroscope stores profiling data"`
	APIBindAddr string `def:":4040" desc:"port for the HTTP server used for data ingestion and web UI"`
	BaseURL     string `def:"" desc:"base URL for when the server is behind a reverse proxy with a different path"`

	// These will eventually be replaced by some sort of a system that keeps track of RAM
	//   and updates
	CacheDimensionSize  int `def:"1000" desc:"max number of elements in LRU cache for dimensions"`
	CacheDictionarySize int `def:"1000" desc:"max number of elements in LRU cache for dictionaries"`
	CacheSegmentSize    int `def:"1000" desc:"max number of elements in LRU cache for segments"`
	CacheTreeSize       int `def:"10000" desc:"max number of elements in LRU cache for trees"`

	// TODO: I don't think a lot of people will change these values.
	//   I think these should just be constants.
	BadgerNoTruncate bool `def:"false" desc:"indicates whether value log files should be truncated to delete corrupt data, if any"`

	MaxNodesSerialization int `def:"2048" desc:"max number of nodes used when saving profiles to disk"`
	MaxNodesRender        int `def:"8192" desc:"max number of nodes used to display data on the frontend"`

	// currently only used in our demo app
	HideApplications []string `def:"" desc:"please don't use, this will soon be deprecated"`

	OutOfSpaceThreshold bytesize.ByteSize `def:"512MB" desc:"Threshold value to consider out of space in bytes"`
	SampleRate          uint              `def:"100" desc:"sample rate for the profiler in Hz. 100 means reading 100 times per second"`
}

type Convert struct {
	Format string `def:"tree"`
}

type DbManager struct {
	LogLevel        string `def:"error" desc:"log level: debug|info|warn|error"`
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
	SampleRate             uint          `def:"100" desc:"sample rate for the profiler in Hz. 100 means reading 100 times per second"`
	DetectSubprocesses     bool          `def:"true" desc:"makes pyroscope keep track of and profile subprocesses of the main process"`
	LogLevel               string        `def:"info" desc:"log level: debug|info|warn|error"`
	ServerAddress          string        `def:"http://localhost:4040" desc:"address of the pyroscope server"`
	AuthToken              string        `def:"" desc:"authorization token used to upload profiling data"`
	UpstreamThreads        int           `def:"4" desc:"number of upload threads"`
	UpstreamRequestTimeout time.Duration `def:"10s" desc:"profile upload timeout"`
	NoLogging              bool          `def:"false" desc:"disables logging from pyroscope"`
	NoRootDrop             bool          `def:"false" desc:"disables permissions drop when ran under root. use this one if you want to run your command as root"`
	Pid                    int           `def:"0" desc:"PID of the process you want to profile. Pass -1 to profile the whole system (only supported by ebpfspy)"`
	UserName               string        `def:"" desc:"starts process under specified user name"`
	GroupName              string        `def:"" desc:"starts process under specified group name"`
	PyspyBlocking          bool          `def:"false" desc:"enables blocking mode for pyspy"`
}
