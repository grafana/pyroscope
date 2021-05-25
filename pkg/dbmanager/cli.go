package dbmanager

import (
	"fmt"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/direct"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/atexit"
	"github.com/sirupsen/logrus"

	"github.com/cheggaaa/pb/v3"
)

func Cli(db_cfg *config.DbManager, srv_cfg *config.Server, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("please provide a command")
	}

	switch args[0] {
	case "copy":
		// TODO: this is meh, I think config.Config should be separate from storage config
		srv_cfg.StoragePath = db_cfg.StoragePath
		srv_cfg.LogLevel = "error"
		copyData(db_cfg, srv_cfg)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}

	return nil
}

// TODO: get this from config or something like that
const resolution = 10 * time.Second

// src start time, src end time, dst start time
func copyData(db_cfg *config.DbManager, srv_cfg *config.Server) error {
	appName := db_cfg.ApplicationName
	srcSt := db_cfg.SrcStartTime.Truncate(resolution)
	dstSt := db_cfg.DstStartTime.Truncate(resolution)
	dstEt := db_cfg.DstEndTime.Truncate(resolution)
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

	s, err := storage.New(srv_cfg)
	if err != nil {
		return err
	}

	if db_cfg.EnableProfiling {
		u := direct.New(s)
		go agent.SelfProfile(100, u, "pyroscope.dbmanager.cpu{}", logrus.StandardLogger())
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
		gOut, err := s.Get(&storage.GetInput{
			StartTime: srct,
			EndTime:   srct2,
			Key:       sk,
		})
		if err != nil {
			return err
		}

		if gOut.Tree != nil {
			dstt := srct.Add(durDiff)
			dstt2 := dstt.Add(resolution)

			err = s.Put(&storage.PutInput{
				StartTime:  dstt,
				EndTime:    dstt2,
				Key:        sk,
				Val:        gOut.Tree,
				SpyName:    gOut.SpyName,
				SampleRate: gOut.SampleRate,
				Units:      gOut.Units,
			})
			if err != nil {
				return err
			}
		}
	}

	bar.Finish()

	s.Close()

	return nil
}
