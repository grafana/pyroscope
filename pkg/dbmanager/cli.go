package dbmanager

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cheggaaa/pb/v3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/pyroscope-io/pyroscope/pkg/exporter"
	"github.com/pyroscope-io/pyroscope/pkg/health"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/direct"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

func Cli(dbCfg *config.DbManager, srvCfg *config.Server, args []string) error {
	switch args[0] {
	case "copy":
		stCfg := storage.NewConfig(srvCfg).WithPath(dbCfg.StoragePath)
		err := copyData(dbCfg, stCfg)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}

	return nil
}

// TODO: get this from config or something like that
const resolution = 10 * time.Second

// src start time, src end time, dst start time
func copyData(dbCfg *config.DbManager, stCfg *storage.Config) error {
	appName := dbCfg.ApplicationName
	srcSt := dbCfg.SrcStartTime.Truncate(resolution)
	dstSt := dbCfg.DstStartTime.Truncate(resolution)
	dstEt := dbCfg.DstEndTime.Truncate(resolution)
	srcEt := srcSt.Add(dstEt.Sub(dstSt))

	fmt.Printf("copying %s from %s-%s to %s-%s\n",
		appName,
		srcSt.String(),
		srcEt.String(),
		dstSt.String(),
		dstEt.String(),
	)

	// TODO: add more correctness checks
	if !srcSt.Before(srcEt) || !dstSt.Before(dstEt) {
		return fmt.Errorf("Incorrect time parameters. Start time has to be before end time. "+
			"src start: %q end: %q, dst start: %q end: %q", srcSt, srcEt, dstSt, dstEt)
	}

	s, err := storage.New(stCfg, logrus.StandardLogger(), prometheus.DefaultRegisterer, new(health.Controller))
	if err != nil {
		return err
	}

	e, _ := exporter.NewExporter(nil, nil)
	if dbCfg.EnableProfiling {
		upstream := direct.New(s, e)
		selfProfilingConfig := agent.SessionConfig{
			Upstream:       upstream,
			AppName:        "pyroscope.dbmanager.cpu{}",
			ProfilingTypes: types.DefaultProfileTypes,
			SpyName:        types.GoSpy,
			SampleRate:     100,
			UploadRate:     10 * time.Second,
			Logger:         logrus.StandardLogger(),
		}
		session, _ := agent.NewSession(selfProfilingConfig)
		upstream.Start()
		_ = session.Start()
	}

	sk, err := segment.ParseKey(appName)
	if err != nil {
		return err
	}

	count := int(srcEt.Sub(srcSt) / (resolution))
	bar := pb.StartNew(count)

	durDiff := dstSt.Sub(srcSt)

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)

loop:
	for srct := srcSt; srct.Before(srcEt); srct = srct.Add(resolution) {
		bar.Increment()
		select {
		case <-sigc:
			break loop
		default:
		}

		srct2 := srct.Add(resolution)
		gOut, err := s.Get(context.TODO(), &storage.GetInput{
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

			err = s.Put(context.TODO(), &storage.PutInput{
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
	return s.Close()
}
