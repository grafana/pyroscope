package testing

import (
	"log"
)

func init() {
	log.SetFlags(log.Lshortfile | log.Ldate | log.Ltime)
}

func SetupLogging() {
	// log.SetFormatter(&log.JSONFormatter{})
}
