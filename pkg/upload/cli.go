package upload

import (
	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"io"
	"os"
	"reflect"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

func Cli(cfg *config.Upload, s *config.Server, logger func(string), args []string) error {
	var input io.Reader
	var err error

	if len(args) == 0 {
		input = os.Stdin
	} else {
		input, err = os.Open(args[0])
		if err != nil {
			return err
		}
	}

	//TODO Change when storage config is refactored out of Server object
	s.StoragePath = cfg.StoragePath
	s.LogLevel = cfg.LogLevel

	tVal := tree.New()
	err = convert.ParseGroups(input, func(name []byte, val int) {
		tVal.Insert(name, uint64(val))
	})
	if err != nil {
		return err
	}

	key, err := storage.ParseKey(cfg.ApplicationName)
	if err != nil {
		return err
	}

	sg, err := storage.New(s)

	if err != nil {
		return err
	}

	// Todo: Introduce validation for Config values ?
	bailIfNotSet(&cfg.StartTime, "StartTime time have to be set")
	bailIfNotSet(&cfg.EndTime, "EndTime have to be set")
	bailIfNotSet(&cfg.AggregationType, "AggregationType have to be set")
	bailIfNotSet(&cfg.SampleRate, "SampleRate have to be set")
	bailIfNotSet(&cfg.SpyName, "SpyName have to be set")
	bailIfNotSet(&cfg.Units, "Units have to be set")

	err = sg.Put(&storage.PutInput{
		Key:             key,
		Val:             tVal,
		StartTime:       cfg.StartTime,
		EndTime:         cfg.EndTime,
		AggregationType: cfg.AggregationType,
		SampleRate:      uint32(cfg.SampleRate),
		SpyName:         cfg.SpyName,
		Units:           cfg.Units,
	})

	return err
}

func bailIfNotSet(val interface{}, msg string) {
	if reflect.ValueOf(val).IsZero() {
		panic(msg)
	}
}
