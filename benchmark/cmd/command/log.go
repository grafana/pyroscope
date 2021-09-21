package command

import (
	"github.com/sirupsen/logrus"
)

func setLogLevel(level string) {
	if l, err := logrus.ParseLevel(level); err == nil {
		logrus.SetLevel(l)
	}
}
