package main

import (
	"log"
	"os"
	"runtime"

	"github.com/fatih/color"
	"github.com/sirupsen/logrus"
)

func init() {
	log.SetFlags(log.Lshortfile | log.Ldate | log.Ltime)

	logrus.SetFormatter(&logrus.TextFormatter{})
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.DebugLevel)

	if runtime.GOOS == "windows" {
		color.NoColor = true
	}
}
