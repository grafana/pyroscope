package pkg

import (
	golog "log"
	"os"

	log "github.com/sirupsen/logrus"
)

func init() {
	golog.SetFlags(golog.Lshortfile | golog.Ldate | golog.Ltime)

	log.SetFormatter(&log.TextFormatter{})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.DebugLevel)
}
