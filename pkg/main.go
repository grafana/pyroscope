package pkg

import (
	"fmt"
	"os"
	"time"

	"github.com/petethepig/pyroscope/pkg/agent"
	"github.com/petethepig/pyroscope/pkg/build"
	"github.com/petethepig/pyroscope/pkg/config"
	"github.com/petethepig/pyroscope/pkg/server"
	"github.com/petethepig/pyroscope/pkg/storage"
	"github.com/petethepig/pyroscope/pkg/util/atexit"
	log "github.com/sirupsen/logrus"
)

func Main() {
	cfg := config.New()
	err := cfg.Load()
	if err != nil {
		log.Info("Failed to load configuration: ", err)
		os.Exit(1)
	}

	// TODO: I think we should split config and flags parsing. I think Config shouldn't know about flags.

	switch cfg.Subcommand {
	case "main":
		if cfg.Version {
			fmt.Println(config.MaybeGradientBanner())
			fmt.Println(build.Summary())
			return
		} else {
			fmt.Println(config.MaybeGradientBanner())
			fmt.Println(cfg.Usage())
		}
	case "server":
		startServer(cfg)
	case "agent":
		agent.New(cfg).Start()
	default:
		log.Fatalf("unknown subcommand: %q", cfg.Subcommand)
	}
}

func startServer(cfg *config.Config) {
	storage, err := storage.New(cfg)
	if err != nil {
		panic(err)
	}
	atexit.Register(storage.Cleanup)
	c := server.New(cfg, storage)
	c.Start()
	time.Sleep(time.Second)
}
