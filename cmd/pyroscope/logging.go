package main

import (
	"log"
	"os"

	"github.com/sirupsen/logrus"
)

func init() {
	log.SetFlags(log.Lshortfile | log.Ldate | log.Ltime)

	logrus.SetFormatter(&logrus.TextFormatter{})
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.DebugLevel)
}
