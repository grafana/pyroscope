package main

import (
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	log "github.com/sirupsen/logrus"
)

func main() {
	cfg := config.New()
	err := cli.Start(cfg)
	if err != nil {
		log.Fatal(err)
	}
}
