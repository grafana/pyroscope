package agent

// Logger is an interface that library users can use
// It is based on logrus, but much smaller — That's because we don't want library users to have to implement
// all of the logrus's methods
type Logger interface {
	Infof(_ string, _ ...interface{})
	Debugf(_ string, _ ...interface{})
	Errorf(_ string, _ ...interface{})
}

type NoopLogger struct{}

func (*NoopLogger) Infof(_ string, _ ...interface{})  {}
func (*NoopLogger) Debugf(_ string, _ ...interface{}) {}
func (*NoopLogger) Errorf(_ string, _ ...interface{}) {}
