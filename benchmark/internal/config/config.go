package config

type Config struct {
	LoadGen             LoadGen             `skip:"true" mapstructure:",squash"`
	PromQuery           PromQuery           `skip:"true" mapstructure:",squash"`
	Report              Report              `skip:"true" mapstructure:",squash"`
	DashboardScreenshot DashboardScreenshot `skip:"true" mapstructure:",squash"`
}

type LoadGen struct {
	LogLevel string `def:"info" desc:"log level: debug|info|warn|error" mapstructure:"log-level"`

	ServerAddress       string `def:"http://localhost:4040" desc:"address of the pyroscope instance being attacked" mapstructure:"server-address"`
	RandSeed            int    `def:"23061912" desc:""`
	ProfileWidth        int    `def:"20"`
	ProfileDepth        int    `def:"20"`
	ProfileSymbolLength int    `def:"30"`
	Fixtures            int    `def:"30" desc:"how many different profiles to generate per app"`
	Apps                int    `def:"20" desc:"how many pyroscope apps to emulate"`
	Clients             int    `def:"20" desc:"how many pyroscope clients to emulate"`
	Requests            int    `def:"10000" desc:"how many requests each clients should make"`

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
	DashboardUid   string `def:"QF9YgRbUbt3BA5Qd" desc:"UUID of the dashboard"`
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
}

type DashboardScreenshot struct {
	LogLevel       string `def:"info" desc:"log level: debug|info|warn|error" mapstructure:"log-level"`
	TimeoutSeconds int    `def:"300" desc:"timeout in seconds of each call"`

	GrafanaAddress string `def:"http://localhost:4050" desc:"address of the grafana instance"`
	DashboardUid   string `def:"QF9YgRbUbt3BA5Qd" desc:"UUID of the dashboard"`
	Destination    string `def:"fs" desc:"where to upload to: s3|fs"`
}

// File can be read from file system.
type File interface{ Path() string }
