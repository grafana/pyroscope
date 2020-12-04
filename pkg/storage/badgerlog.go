package storage

import log "github.com/sirupsen/logrus"

type badgerLogger struct{}

func (b badgerLogger) Errorf(firstArg string, args ...interface{}) {
	log.WithField("context", "badger").Errorf(firstArg, args...)
}

func (b badgerLogger) Warningf(firstArg string, args ...interface{}) {
	log.WithField("context", "badger").Warningf(firstArg, args...)
}

func (b badgerLogger) Infof(firstArg string, args ...interface{}) {
	log.WithField("context", "badger").Infof(firstArg, args...)
}

func (b badgerLogger) Debugf(firstArg string, args ...interface{}) {
	log.WithField("context", "badger").Debugf(firstArg, args...)
}
