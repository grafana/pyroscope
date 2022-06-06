package direct

import (
	"bytes"
	"context"
	"runtime/debug"

	"github.com/pyroscope-io/client/upstream"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/convert/pprof"
	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/pyroscope-io/pyroscope/pkg/parser"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type Direct struct {
	logger logrus.FieldLogger
	parser *parser.Parser
}

func New(logger logrus.FieldLogger, p *parser.Parser) *Direct {
	return &Direct{
		logger: logger,
		parser: p,
	}
}

func (u *Direct) Upload(j *upstream.UploadJob) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("panic recovered: %v; %v", r, string(debug.Stack()))
		}
	}()

	key, err := segment.ParseKey(j.Name)
	logger := u.logger.WithField("key", key)
	if err != nil {
		logger.Error("invalid key")
		return
	}

	if len(j.Profile) == 0 {
		logger.Warn("empty profile")
		return
	}

	profile := &pprof.RawProfile{
		Profile:          bytes.NewBuffer(j.Profile),
		SampleTypeConfig: tree.DefaultSampleTypeMapping,
	}
	if len(j.PrevProfile) > 0 {
		profile.PreviousProfile = bytes.NewBuffer(j.PrevProfile)
	}

	err = u.parser.Ingest(context.TODO(), &ingestion.IngestInput{
		Format:  ingestion.FormatPprof,
		Profile: profile,
		Metadata: ingestion.Metadata{
			SpyName:   j.SpyName,
			StartTime: j.StartTime,
			EndTime:   j.EndTime,
			Key:       key,
		},
	})

	if err != nil {
		logger.WithError(err).Error("failed to store a local profile")
	}
}
