package config

import (
	"os"

	"github.com/sirupsen/logrus"
)

type FileConfiger interface{ ConfigFilePath() string }

func (cfg Agent) ConfigFilePath() string {
	return cfg.Config
}

func (cfg Server) ConfigFilePath() string {
	return cfg.Config
}

func (cfg CombinedDbManager) ConfigFilePath() string {
	return cfg.Server.Config
}

type LoggerFunc func(s string)
type LoggerConfiger interface{ InitializeLogging() LoggerFunc }

func (cfg Convert) InitializeLogging() LoggerFunc {
	logrus.SetOutput(os.Stderr)
	logger := func(s string) {
		logrus.Fatal(s)
	}

	return logger
}

func (cfg CombinedDbManager) InitializeLogging() LoggerFunc {
	if l, err := logrus.ParseLevel(cfg.DbManager.LogLevel); err == nil {
		logrus.SetLevel(l)
	}

	return nil
}

func (cfg Exec) InitializeLogging() LoggerFunc {
	if cfg.NoLogging {
		logrus.SetLevel(logrus.PanicLevel)
	} else if l, err := logrus.ParseLevel(cfg.LogLevel); err == nil {
		logrus.SetLevel(l)
	}

	return nil
}
