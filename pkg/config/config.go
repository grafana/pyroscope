package config

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

type Config struct {
	Version bool `mapstructure:"version"`

	Agent     Agent     `skip:"true" mapstructure:",squash"`
	Server    Server    `skip:"true" mapstructure:",squash"`
	Convert   Convert   `skip:"true" mapstructure:",squash"`
	Exec      Exec      `skip:"true" mapstructure:",squash"`
	DbManager DbManager `skip:"true" mapstructure:",squash"`
}

type Agent struct {
	Config string `def:"<defaultAgentConfigPath>" desc:"location of config file" mapstructure:"config"`

	LogFilePath string `def:"<defaultAgentLogFilePath>" desc:"log file path" mapstructure:"log-file-path"`
	LogLevel    string `def:"info" desc:"log level: debug|info|warn|error" mapstructure:"log-level"`
	NoLogging   bool   `def:"false" desc:"disables logging from pyroscope" mapstructure:"no-logging"`

	ServerAddress          string        `def:"http://localhost:4040" desc:"address of the pyroscope server" mapstructure:"server-address"`
	AuthToken              string        `def:"" desc:"authorization token used to upload profiling data" mapstructure:"auth-token"`
	UpstreamThreads        int           `def:"4" desc:"number of upload threads" mapstructure:"upstream-threads"`
	UpstreamRequestTimeout time.Duration `def:"10s" desc:"profile upload timeout" mapstructure:"upstream-request-timeout"`

	// Structs and slices are not parsed with ffcli package,
	// instead `loadAgentConfig` function should be used.
	Targets []Target `yaml:"targets" desc:"list of targets to be profiled" mapstructure:"targets"`

	// Note that in YAML the key is 'tags' but the flag is 'tag'.
	Tags map[string]string `yaml:"tags" name:"tag" def:"" desc:"tag key value pairs" mapstructure:"tags"`
}

type Target struct {
	ServiceName string `yaml:"service-name" desc:"name of the system service to be profiled"`

	SpyName            string `yaml:"spy-name" def:"" desc:"name of the profiler you want to use. Supported ones are: <supportedProfilers>"`
	ApplicationName    string `yaml:"application-name" def:"" desc:"application name used when uploading profiling data"`
	SampleRate         uint   `yaml:"sample-rate" def:"100" desc:"sample rate for the profiler in Hz. 100 means reading 100 times per second"`
	DetectSubprocesses bool   `yaml:"detect-subprocesses" def:"true" desc:"makes pyroscope keep track of and profile subprocesses of the main process"`

	// Spy-specific settings.
	PyspyBlocking bool `yaml:"pyspy-blocking" def:"false" desc:"enables blocking mode for pyspy"`
	RbspyBlocking bool `yaml:"rbspy-blocking" def:"false" desc:"enables blocking mode for rbspy"`

	// Tags are inherited from the agent level. At some point we may need
	// specifying tags at the target level (override).
	Tags map[string]string `yaml:"-"`
}

type Server struct {
	AnalyticsOptOut bool `def:"false" desc:"disables analytics" mapstructure:"analytics-opt-out"`

	Config         string `def:"<installPrefix>/etc/pyroscope/server.yml" desc:"location of config file" mapstructure:"config"`
	LogLevel       string `def:"info" desc:"log level: debug|info|warn|error" mapstructure:"log-level"`
	BadgerLogLevel string `def:"error" desc:"log level: debug|info|warn|error" mapstructure:"badger-log-level"`

	StoragePath string `def:"<installPrefix>/var/lib/pyroscope" desc:"directory where pyroscope stores profiling data" mapstructure:"storage-path"`
	APIBindAddr string `def:":4040" desc:"port for the HTTP server used for data ingestion and web UI" mapstructure:"api-bind-addr"`
	BaseURL     string `def:"" desc:"base URL for when the server is behind a reverse proxy with a different path" mapstructure:"base-url"`

	CacheEvictThreshold float64 `def:"0.25" desc:"percentage of memory at which cache evictions start" mapstructure:"cache-evict-threshold"`
	CacheEvictVolume    float64 `def:"0.33" desc:"percentage of cache that is evicted per eviction run" mapstructure:"cache-evict-volume"`

	// TODO: I don't think a lot of people will change these values.
	//   I think these should just be constants.
	BadgerNoTruncate     bool `def:"false" desc:"indicates whether value log files should be truncated to delete corrupt data, if any" mapstructure:"badger-no-truncate"`
	DisablePprofEndpoint bool `def:"false" desc:"disables /debug/pprof route" mapstructure:"disable-pprof-endpoint"`

	MaxNodesSerialization int `def:"2048" desc:"max number of nodes used when saving profiles to disk" mapstructure:"max-nodes-serialization"`
	MaxNodesRender        int `def:"8192" desc:"max number of nodes used to display data on the frontend" mapstructure:"max-nodes-render"`

	// currently only used in our demo app
	HideApplications []string `def:"" desc:"please don't use, this will soon be deprecated" mapstructure:"hide-applications"`

	Retention time.Duration `def:"" desc:"sets the maximum amount of time the profiling data is stored for. Data before this threshold is deleted. Disabled by default" mapstructure:"retention"`

	// Deprecated fields. They can be set (for backwards compatibility) but have no effect
	// TODO: we should print some warning messages when people try to use these
	SampleRate          uint              `deprecated:"true" mapstructure:"sample-rate"`
	OutOfSpaceThreshold bytesize.ByteSize `deprecated:"true" mapstructure:"out-of-space-threshold"`
	CacheDimensionSize  int               `deprecated:"true" mapstructure:"cache-dimensions-size"`
	CacheDictionarySize int               `deprecated:"true" mapstructure:"cache-dictonary-size"`
	CacheSegmentSize    int               `deprecated:"true" mapstructure:"cache-segment-size"`
	CacheTreeSize       int               `deprecated:"true" mapstructure:"cache-tree-size"`

	Auth Auth `mapstructure:"auth"`

	MetricExportRules MetricExportRules `yaml:"metric-export-rules" def:"" desc:"metric export rules" mapstructure:"metric-export-rules"`
}

type MetricExportRules map[string]MetricExportRule

type MetricExportRule struct {
	Expr   string   `def:"" desc:"expression in FlameQL syntax to be evaluated against samples" mapstructure:"expr"`
	Node   string   `def:"total" desc:"tree node filter expression. Should be either 'total' or a valid regexp" mapstructure:"node"`
	Labels []string `def:"" desc:"list of tags to be exported as prometheus labels" mapstructure:"labels"`
}

type Auth struct {
	Google GoogleOauth `mapstructure:"google"`
	Gitlab GitlabOauth `mapstructure:"gitlab"`
	Github GithubOauth `mapstructure:"github"`

	// TODO: can we generate these automatically if it's empty?
	JWTSecret                string `json:"-" deprecated:"true" def:"" desc:"secret used to secure your JWT tokens" mapstructure:"jwt-secret"`
	LoginMaximumLifetimeDays int    `json:"-" deprecated:"true" def:"0" desc:"amount of days after which user will be logged out. 0 means non-expiring." mapstructure:"login-maximum-lifetime-days"`
}

// TODO: Maybe merge Oauth structs into one (would have to move def and desc tags somewhere else in code)
type GoogleOauth struct {
	// TODO: remove deprecated: true when we enable these back
	Enabled        bool     `json:"-" deprecated:"true" def:"false" desc:"enables Google Oauth" mapstructure:"enabled"`
	ClientID       string   `json:"-" deprecated:"true" def:"" desc:"client ID generated for Google API" mapstructure:"client-id"`
	ClientSecret   string   `json:"-" deprecated:"true" def:"" desc:"client secret generated for Google API" mapstructure:"client-secret"`
	RedirectURL    string   `json:"-" deprecated:"true" def:"" desc:"url that google will redirect to after logging in. Has to be in form <pathToPyroscopeServer/auth/google/callback>" mapstructure:"redirect-url"`
	AuthURL        string   `json:"-" deprecated:"true" def:"https://accounts.google.com/o/oauth2/auth" desc:"auth url for Google API (usually present in credentials.json file)" mapstructure:"auth-url"`
	TokenURL       string   `json:"-" deprecated:"true" def:"https://accounts.google.com/o/oauth2/token" desc:"token url for Google API (usually present in credentials.json file)" mapstructure:"token-url"`
	AllowedDomains []string `json:"-" deprecated:"true" def:"" desc:"list of domains that are allowed to login through google" mapstructure:"allowed-domains"`
}

type GitlabOauth struct {
	Enabled bool `json:"-" deprecated:"true" def:"false" desc:"enables Gitlab Oauth" mapstructure:"enabled"`
	// TODO: I changed this to ClientID to fit others, but in Gitlab docs it's Application ID so it might get someone confused?
	ClientID      string   `json:"-" deprecated:"true" def:"" desc:"client ID generated for GitLab API" mapstructure:"client-id"`
	ClientSecret  string   `json:"-" deprecated:"true" def:"" desc:"client secret generated for GitLab API" mapstructure:"client-secret"`
	RedirectURL   string   `json:"-" deprecated:"true" def:"" desc:"url that gitlab will redirect to after logging in. Has to be in form <pathToPyroscopeServer/auth/gitlab/callback>" mapstructure:"redirect-url"`
	AuthURL       string   `json:"-" deprecated:"true" def:"https://gitlab.com/oauth/authorize" desc:"auth url for GitLab API (keep default for cloud, usually https://gitlab.mycompany.com/oauth/authorize for on-premise)" mapstructure:"auth-url"`
	TokenURL      string   `json:"-" deprecated:"true" def:"https://gitlab.com/oauth/token" desc:"token url for GitLab API (keep default for cloud, usually https://gitlab.mycompany.com/oauth/token for on-premise)" mapstructure:"token-url"`
	APIURL        string   `json:"-" deprecated:"true" def:"https://gitlab.com/api/v4" desc:"URL to gitlab API (keep default for cloud, usually https://gitlab.mycompany.com/api/v4/user for on-premise)" mapstructure:"api-url"`
	AllowedGroups []string `json:"-" deprecated:"true" def:"" desc:"list of groups (unique names of the group as listed in URL) that are allowed to login through gitlab" mapstructure:"allowed-groups"`
}

type GithubOauth struct {
	Enabled              bool     `json:"-" deprecated:"true" def:"false" desc:"enables Github Oauth" mapstructure:"enabled"`
	ClientID             string   `json:"-" deprecated:"true" def:"" desc:"client ID generated for Github API" mapstructure:"client-id"`
	ClientSecret         string   `json:"-" deprecated:"true" def:"" desc:"client secret generated for Github API" mapstructure:"client-secret"`
	RedirectURL          string   `json:"-" deprecated:"true" def:"" desc:"url that Github will redirect to after logging in. Has to be in form <pathToPyroscopeServer/auth/github/callback>" mapstructure:"redirect-url"`
	AuthURL              string   `json:"-" deprecated:"true" def:"https://github.com/login/oauth/authorize" desc:"auth url for Github API" mapstructure:"auth-url"`
	TokenURL             string   `json:"-" deprecated:"true" def:"https://github.com/login/oauth/access_token" desc:"token url for Github API" mapstructure:"token-url"`
	AllowedOrganizations []string `json:"-" deprecated:"true" def:"" desc:"list of organizations that are allowed to login through github" mapstructure:"allowed-organizations"`
}

type Convert struct {
	Format string `def:"tree" mapstructure:"format"`
}

type CombinedDbManager struct {
	*DbManager `mapstructure:",squash"`
	*Server    `mapstructure:",squash"`
}

type DbManager struct {
	LogLevel        string `def:"error" desc:"log level: debug|info|warn|error" mapstructure:"log-level"`
	StoragePath     string `def:"<installPrefix>/var/lib/pyroscope" desc:"directory where pyroscope stores profiling data" mapstructure:"storage-path"`
	DstStartTime    time.Time
	DstEndTime      time.Time
	SrcStartTime    time.Time
	ApplicationName string

	EnableProfiling bool `def:"false" desc:"enables profiling of dbmanager" mapstructure:"enable-profiling"`
}

type Exec struct {
	SpyName                string        `def:"auto" desc:"name of the profiler you want to use. Supported ones are: <supportedProfilers>" mapstructure:"spy-name"`
	ApplicationName        string        `def:"" desc:"application name used when uploading profiling data" mapstructure:"application-name"`
	SampleRate             uint          `def:"100" desc:"sample rate for the profiler in Hz. 100 means reading 100 times per second" mapstructure:"sample-rate"`
	DetectSubprocesses     bool          `def:"true" desc:"makes pyroscope keep track of and profile subprocesses of the main process" mapstructure:"detect-subprocesses"`
	LogLevel               string        `def:"info" desc:"log level: debug|info|warn|error" mapstructure:"log-level"`
	ServerAddress          string        `def:"http://localhost:4040" desc:"address of the pyroscope server" mapstructure:"server-address"`
	AuthToken              string        `def:"" desc:"authorization token used to upload profiling data" mapstructure:"auth-token"`
	UpstreamThreads        int           `def:"4" desc:"number of upload threads" mapstructure:"upstream-threads"`
	UpstreamRequestTimeout time.Duration `def:"10s" desc:"profile upload timeout" mapstructure:"upstream-request-timeout"`
	NoLogging              bool          `def:"false" desc:"disables logging from pyroscope" mapstructure:"no-logging"`
	NoRootDrop             bool          `def:"false" desc:"disables permissions drop when ran under root. use this one if you want to run your command as root" mapstructure:"no-root-drop"`
	Pid                    int           `def:"0" desc:"PID of the process you want to profile. Pass -1 to profile the whole system (only supported by ebpfspy)" mapstructure:"pid"`
	UserName               string        `def:"" desc:"starts process under specified user name" mapstructure:"user-name"`
	GroupName              string        `def:"" desc:"starts process under specified group name" mapstructure:"group-name"`
	PyspyBlocking          bool          `def:"false" desc:"enables blocking mode for pyspy" mapstructure:"pyspy-blocking"`
	RbspyBlocking          bool          `def:"false" desc:"enables blocking mode for rbspy" mapstructure:"rbspy-blocking"`

	Tags map[string]string `name:"tag" def:"" desc:"tag in key=value form. The flag may be specified multiple times" mapstructure:"tags"`
}
