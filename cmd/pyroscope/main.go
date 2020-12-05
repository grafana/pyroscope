package main

import (
	"github.com/petethepig/pyroscope/cmd/pyroscope/cli"
	"github.com/petethepig/pyroscope/pkg/config"
	log "github.com/sirupsen/logrus"
)

func main() {
	cfg := config.New()
	err := cli.Start(cfg)
	if err != nil {
		log.Fatal(err)
	}
}
