package main

import (
	"context"
	"os"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"

	pushv1 "github.com/grafana/phlare/api/gen/proto/go/push/v1"
	"github.com/grafana/phlare/api/gen/proto/go/push/v1/pushv1connect"
	"github.com/grafana/phlare/pkg/model"
	"github.com/grafana/phlare/pkg/pprof"
)

func (c *phlareClient) pusherClient() pushv1connect.PusherServiceClient {
	return pushv1connect.NewPusherServiceClient(
		c.httpClient(),
		c.URL,
	)
}

type uploadParams struct {
	*phlareClient
	paths       []string
	extraLabels map[string]string
}

func addUploadParams(cmd commander) *uploadParams {
	var (
		params = &uploadParams{
			extraLabels: map[string]string{},
		}
	)
	params.phlareClient = addPhlareClient(cmd)

	cmd.Arg("path", "Path(s) to profile(s) to upload").Required().ExistingFilesVar(&params.paths)
	cmd.Flag("extra-labels", "Add additional labels to the profile(s)").Default("job=profilecli-upload").StringMapVar(&params.extraLabels)
	return params
}

func upload(ctx context.Context, params *uploadParams) (err error) {
	pc := params.phlareClient.pusherClient()

	lblStrings := make([]string, 0, len(params.extraLabels)*2)
	for key, value := range params.extraLabels {
		lblStrings = append(lblStrings, key, value)
	}

	var (
		lbl        = model.LabelsFromStrings(lblStrings...)
		series     = make([]*pushv1.RawProfileSeries, len(params.paths))
		lblBuilder = model.NewLabelsBuilder(lbl)
	)
	for idx, path := range params.paths {
		lblBuilder.Reset(lbl)

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		profile, err := pprof.RawFromBytes(data)
		if err != nil {
			return err
		}

		// detect name if no name has been set
		if lbl.Get(model.LabelNameProfileName) == "" {
			name := "unknown"
			for _, t := range profile.Profile.SampleType {
				if sid := int(t.Type); sid < len(profile.StringTable) {
					if s := profile.StringTable[sid]; s == "cpu" {
						name = "process_cpu"
						break
					} else if s == "alloc_space" || s == "inuse_space" {
						name = "memory"
						break
					} else {
						level.Debug(logger).Log("msg", "unspecific/unknown profile sample type", "profile", s)
					}
				}
			}
			lblBuilder.Set(model.LabelNameProfileName, name)
		}

		series[idx] = &pushv1.RawProfileSeries{
			Labels: lblBuilder.Labels(),
			Samples: []*pushv1.RawSample{{
				ID:         uuid.New().String(),
				RawProfile: data,
			}},
		}
	}

	_, err = pc.Push(ctx, connect.NewRequest(&pushv1.PushRequest{
		Series: series,
	}))

	if err != nil {
		return err
	}

	for idx := range series {
		level.Info(logger).Log("msg", "successfully uploaded profile", "id", series[idx].Samples[0].ID, "labels", model.Labels(series[idx].Labels).ToPrometheusLabels().String(), "path", params.paths[idx])
	}

	return nil
}
