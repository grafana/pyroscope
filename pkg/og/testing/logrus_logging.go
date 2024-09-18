package testing

import (
	"os"

	golog "log"

	"github.com/sirupsen/logrus"
)

func init() {
	golog.SetFlags(golog.Lshortfile | golog.Ldate | golog.Ltime)
}

func SetupLogging() {
	// log.SetFormatter(&log.JSONFormatter{})
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.DebugLevel)
}
