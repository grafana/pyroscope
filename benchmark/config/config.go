package config

type Config struct {
	LoadGen   LoadGen   `skip:"true" mapstructure:",squash"`
	PromQuery PromQuery `skip:"true" mapstructure:",squash"`
	CIReport  CIReport  `skip:"true" mapstructure:",squash"`
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

	WaitUntilAvailable bool `def:"true" desc:"wait until endpoint is available"`
}

type PromQuery struct {
	PrometheusAddress string `def:"http://localhost:9090" desc:"address of the prometheus instance being queried" mapstructure:"server-address"`
}

type CIReport struct {
	PrometheusAddress string `def:"http://localhost:9090" desc:"address of the prometheus instance being queried" mapstructure:"server-address"`
}

// File can be read from file system.
type File interface{ Path() string }
