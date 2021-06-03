package agent

// Logger is an interface that library users can use
// It is based on logrus, but much smaller — That's because we don't want library users to have to implement
// all of the logrus's methods
type Logger interface {
	Infof(format string, args ...interface{})
	Debugf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

type NoopLogger struct{}

func (*NoopLogger) Infof(format string, args ...interface{})  {}
func (*NoopLogger) Debugf(format string, args ...interface{}) {}
func (*NoopLogger) Errorf(format string, args ...interface{}) {}
