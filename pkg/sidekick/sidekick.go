package sidekick

import (
	"context"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/services"

	"github.com/grafana/pyroscope/pkg/frontend"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/sidekick/adhocprofiles"
)

type Sidekick struct {
	services.Service

	AdHocProfiles *adhocprofiles.AdHocProfiles
}

func NewSidekick(bucket objstore.Bucket, logger log.Logger, limits frontend.Limits) *Sidekick {
	s := &Sidekick{
		AdHocProfiles: adhocprofiles.NewAdHocProfiles(bucket, logger, limits),
	}
	s.Service = services.NewBasicService(nil, s.running, nil)
	return s
}

func (s *Sidekick) running(ctx context.Context) error {
	<-ctx.Done()
	return nil
}
