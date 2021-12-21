package config

import "time"

//revive:disable:line-length-limit Most of line length is documentation
//revive:disable:max-public-structs Config structs
type Config struct {
	LoadGen             LoadGen             `skip:"true" mapstructure:",squash"`
	PromQuery           PromQuery           `skip:"true" mapstructure:",squash"`
	Report              Report              `skip:"true" mapstructure:",squash"`
	DashboardScreenshot DashboardScreenshot `skip:"true" mapstructure:",squash"`
}

type LoadGen struct {
	LogLevel string `def:"info" desc:"log level: debug|info|warn|error" mapstructure:"log-level"`

	ServerAddress       string        `def:"http://localhost:4040" desc:"address of the pyroscope instance being attacked" mapstructure:"server-address"`
	RandSeed            int           `def:"23061912" mapstructure:"rand-seed"`
	RealTime            bool          `def:"false" desc:"uploads profiles on 10 second intervals"  mapstructure:"real-time"`
	TimeMultiplier      int           `def:"1" desc:"the higher the value the lower the time between uploads. only used when real-time=true"  mapstructure:"time-multiplier"`
	ProfileWidth        int           `def:"20" mapstructure:"profile-width"`
	ProfileDepth        int           `def:"20" mapstructure:"profile-depth"`
	ProfileSymbolLength int           `def:"30" mapstructure:"profile-symbol-length"`
	Fixtures            int           `def:"30" desc:"how many different profiles to generate per app" mapstructure:"fixtures"`
	Apps                int           `def:"20" desc:"how many pyroscope apps to emulate" mapstructure:"apps"`
	Clients             int           `def:"20" desc:"how many pyroscope clients to emulate" mapstructure:"clients"`
	Requests            int           `def:"10000" desc:"how many requests each clients should make" mapstructure:"requests"`
	Duration            time.Duration `def:"0s" desc:"specifies how long the simulated time segment will be. if set, requests value is calculated based on duration" mapstructure:"duration"`
	TagKeys             int           `def:"2" desc:"how many unique tag keys each app should have" mapstructure:"tag-keys"`
	TagValues           int           `def:"2" desc:"how many unique tag values each app should have" mapstructure:"tag-values"`

	PushgatewayAddress string `def:"" desc:"if enabled, pushes data to prometheus pushgateway (assumes it's unauthenticated)" mapstructure:"pushgateway-address"`
	WaitUntilAvailable bool   `def:"true" desc:"wait until endpoint is available"`
}

type PromQuery struct {
	PrometheusAddress string `def:"http://localhost:9090" desc:"address of the prometheus instance being queried" mapstructure:"server-address"`
}

type Report struct {
	TableReport
	ImageReport
	MetaReport
}

type TableReport struct {
	PrometheusAddress string `def:"http://localhost:9090" desc:"address of the prometheus instance being queried" mapstructure:"server-address"`
	QueriesFile       string `def:"<defaultQueriesFile>" desc:"filepath of the queries file"`
	LogLevel          string `def:"info" desc:"log level: debug|info|warn|error" mapstructure:"log-level"`
}

type ImageReport struct {
	GrafanaAddress string `def:"http://localhost:4050" desc:"address of the grafana instance"`
	DashboardUID   string `def:"QF9YgRbUbt3BA5Qd" desc:"UUID of the dashboard"`
	UploadType     string `def:"fs" desc:"where to upload to: s3|fs" mapstructure:"upload-type"`
	UploadBucket   string `def:"" desc:"bucket name if applicable" mapstructure:"upload-bucket"`
	UploadDest     string `def:"dashboard-screenshots" desc:"name of the output directory" mapstructure:"upload-dest"`
	TimeoutSeconds int    `def:"300" desc:"timeout in seconds of each call"`
	LogLevel       string `def:"info" desc:"log level: debug|info|warn|error" mapstructure:"log-level"`
	From           int    `def:"0" desc:"timestamp"`
	To             int    `def:"0" desc:"timestamp"`
}

type MetaReport struct {
	LogLevel string   `def:"info" desc:"log level: debug|info|warn|error" mapstructure:"log-level"`
	Params   []string `def:"" desc:"the parameters in format A=B. value must be in the allowlist"`
	Title    string   `def:"Server Benchmark" desc:"title for the markdown report"`
}

type DashboardScreenshot struct {
	LogLevel       string `def:"info" desc:"log level: debug|info|warn|error" mapstructure:"log-level"`
	TimeoutSeconds int    `def:"300" desc:"timeout in seconds of each call"`

	GrafanaAddress string `def:"http://localhost:4050" desc:"address of the grafana instance"`
	DashboardUID   string `def:"QF9YgRbUbt3BA5Qd" desc:"UUID of the dashboard"`
	Destination    string `def:"fs" desc:"where to upload to: s3|fs"`
}
