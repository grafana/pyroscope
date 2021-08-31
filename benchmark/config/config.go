package config

type Config struct {
	Version bool `mapstructure:"version"`
}

type LoggerFunc func(s string)
type LoggerConfiger interface{ InitializeLogging() LoggerFunc }
type FileConfiger interface{ ConfigFilePath() string }
