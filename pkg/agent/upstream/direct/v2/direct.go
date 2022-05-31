package direct

import (
	"bytes"
	"context"
	"runtime/debug"

	"github.com/pyroscope-io/client/upstream"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/parser"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
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
	pi := parser.PutInput{
		Format:           parser.Format(j.Format),
		Profile:          bytes.NewReader(j.Profile),
		SampleTypeConfig: tree.DefaultSampleTypeMapping,
		StartTime:        j.StartTime,
		EndTime:          j.EndTime,
		Key:              key,
		SpyName:          j.SpyName,
		SampleRate:       j.SampleRate,
		Units:            metadata.Units(j.Units),
		AggregationType:  metadata.AggregationType(j.AggregationType),
	}
	if len(j.PrevProfile) > 0 {
		pi.PreviousProfile = bytes.NewReader(j.PrevProfile)
	}

	if err = u.parser.Put(context.TODO(), &pi); err != nil {
		logger.WithError(err).Error("failed to store a local profile")
	}
}
