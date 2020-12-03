package testing

import (
	"os"

	golog "log"

	log "github.com/sirupsen/logrus"
)

func init() {
	golog.SetFlags(golog.Lshortfile | golog.Ldate | golog.Ltime)
}

func SetupLogging() {
	// log.SetFormatter(&log.JSONFormatter{})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.DebugLevel)
}
