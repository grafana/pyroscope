package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	gprofile "github.com/google/pprof/profile"
	"github.com/grafana/dskit/runutil"
	"github.com/k0kubun/pp/v3"
	"github.com/klauspost/compress/gzip"
	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

const (
	outputConsole = "console"
	outputRaw     = "raw"
	outputPprof   = "pprof="
)

func outputSeries(result []*typesv1.Labels) error {
	enc := json.NewEncoder(os.Stdout)
	m := make(map[string]interface{})
	for _, s := range result {
		clear(m)
		for _, l := range s.Labels {
			m[l.Name] = l.Value
		}
		if err := enc.Encode(m); err != nil {
			return err
		}
	}
	return nil
}

func outputMergeProfile(ctx context.Context, outputFlag string, profile *googlev1.Profile) error {
	mypp := pp.New()
	mypp.SetColoringEnabled(isatty.IsTerminal(os.Stdout.Fd()))
	mypp.SetExportedOnly(true)

	if outputFlag == outputConsole {
		buf, err := profile.MarshalVT()
		if err != nil {
			return errors.Wrap(err, "failed to marshal protobuf")
		}

		p, err := gprofile.Parse(bytes.NewReader(buf))
		if err != nil {
			return errors.Wrap(err, "failed to parse profile")
		}

		fmt.Fprintln(output(ctx), p.String())
		return nil

	}

	if outputFlag == outputRaw {
		mypp.Print(profile)
		return nil
	}

	if strings.HasPrefix(outputFlag, outputPprof) {
		filePath := strings.TrimPrefix(outputFlag, outputPprof)
		if filePath == "" {
			return errors.New("no file path specified after pprof=")
		}
		buf, err := profile.MarshalVT()
		if err != nil {
			return errors.Wrap(err, "failed to marshal protobuf")
		}

		// open new file, fail when the file already exists
		f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return errors.Wrap(err, "failed to create pprof file")
		}
		defer runutil.CloseWithErrCapture(&err, f, "failed to close pprof file")

		gzipWriter := gzip.NewWriter(f)
		defer runutil.CloseWithErrCapture(&err, gzipWriter, "failed to close pprof gzip writer")

		if _, err := io.Copy(gzipWriter, bytes.NewReader(buf)); err != nil {
			return errors.Wrap(err, "failed to write pprof")
		}

		return nil
	}

	return errors.Errorf("unknown output %s", outputFlag)
}
