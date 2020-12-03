package storage

import log "github.com/sirupsen/logrus"

type badgerLogger struct{}

func (b badgerLogger) Errorf(firstArg string, args ...interface{}) {
	log.Errorf(firstArg, args...)
}

func (b badgerLogger) Warningf(firstArg string, args ...interface{}) {
	log.Warningf(firstArg, args...)
}

func (b badgerLogger) Infof(firstArg string, args ...interface{}) {
	log.Infof(firstArg, args...)
}

func (b badgerLogger) Debugf(firstArg string, args ...interface{}) {
	log.Debugf(firstArg, args...)
}
