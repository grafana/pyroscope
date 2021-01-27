package dbmanager

import (
	"fmt"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/direct"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/atexit"

	"github.com/cheggaaa/pb/v3"
)

func Cli(cfg *config.Config, args []string) error {
	// spew.Dump(cfg.DbManager)
	// spew.Dump(args)

	if len(args) == 0 {
		return fmt.Errorf("please provide a command")
	}

	switch args[0] {
	case "copy":
		copyData(cfg)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}

	return nil
}

// TODO: get this from config or something like that
const resolution = 10 * time.Second

// src start time, src end time, dst start time
func copyData(cfg *config.Config) error {
	// TODO: this is meh, I think config.Config should be separate from storage config
	cfg.Server.StoragePath = cfg.DbManager.StoragePath
	cfg.Server.LogLevel = "error"
	appName := cfg.DbManager.ApplicationName
	srcSt := cfg.DbManager.SrcStartTime.Truncate(resolution)
	dstSt := cfg.DbManager.DstStartTime.Truncate(resolution)
	dstEt := cfg.DbManager.DstEndTime.Truncate(resolution)
	srcEt := srcSt.Add(dstEt.Sub(dstSt))

	fmt.Printf("copying %s from %s-%s to %s-%s\n",
		appName,
		srcSt.String(),
		srcEt.String(),
		dstSt.String(),
		dstEt.String(),
	)

	// TODO: add more correctness checks
	if !srcSt.Before(srcEt) {
		return fmt.Errorf("src start time (%q) has to be before src end time (%q)", srcSt, srcEt)
	}

	if !srcSt.Before(srcEt) {
		return fmt.Errorf("src start time (%q) has to be before src end time (%q)", srcSt, srcEt)
	}

	s, err := storage.New(cfg)
	if err != nil {
		return err
	}

	if cfg.DbManager.EnableProfiling {
		u := direct.New(cfg, s)
		go agent.SelfProfile(cfg, u, "pyroscope.dbmanager.cpu{}")
	}

	st := srcSt
	et := srcEt
	sk, err := storage.ParseKey(appName)
	if err != nil {
		return err
	}

	count := int(et.Sub(st) / (resolution))
	bar := pb.StartNew(count)

	durDiff := dstSt.Sub(srcSt)

	stop := false
	atexit.Register(func() {
		stop = true
	})

	for srct := st; srct.Before(et); srct = srct.Add(resolution) {
		bar.Increment()

		if stop {
			break
		}

		srct2 := srct.Add(resolution)
		tree, _, sn, sr, err := s.Get(srct, srct2, sk)
		if err != nil {
			return err
		}

		if tree != nil {
			dstt := srct.Add(durDiff)
			dstt2 := dstt.Add(resolution)
			err = s.Put(dstt, dstt2, sk, tree, sn, sr)
			if err != nil {
				return err
			}
		}
	}

	bar.Finish()

	s.Close()

	return nil
}
