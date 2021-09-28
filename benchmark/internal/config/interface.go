package config

import (
	"github.com/sirupsen/logrus"
)

type FileConfiger interface{ ConfigFilePath() string }

type LoggerFunc func(s string)
type LoggerConfiger interface{ InitializeLogging() LoggerFunc }

func (cfg LoadGen) InitializeLogging() LoggerFunc {
	if l, err := logrus.ParseLevel(cfg.LogLevel); err == nil {
		logrus.SetLevel(l)
	}

	return nil
}
