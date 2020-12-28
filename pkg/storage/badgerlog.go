package storage

import "github.com/sirupsen/logrus"

type badgerLogger struct {
	name     string
	logLevel logrus.Level
}

func (b badgerLogger) Errorf(firstArg string, args ...interface{}) {
	if b.logLevel < logrus.ErrorLevel {
		return
	}
	logrus.WithField("badger", b.name).Errorf(firstArg, args...)
}

func (b badgerLogger) Warningf(firstArg string, args ...interface{}) {
	if b.logLevel < logrus.WarnLevel {
		return
	}
	logrus.WithField("badger", b.name).Warningf(firstArg, args...)
}

func (b badgerLogger) Infof(firstArg string, args ...interface{}) {
	if b.logLevel < logrus.InfoLevel {
		return
	}
	logrus.WithField("badger", b.name).Infof(firstArg, args...)
}

func (b badgerLogger) Debugf(firstArg string, args ...interface{}) {
	if b.logLevel < logrus.DebugLevel {
		return
	}
	logrus.WithField("badger", b.name).Debugf(firstArg, args...)
}
