package storage

import (
	"context"

	"github.com/pyroscope-io/pyroscope/pkg/model/appmetadata"
)

// NoopApplicationMetadataService implements same methods as ApplicationMetadataService
// But it doesn't do anything when called
type NoopApplicationMetadataService struct{}

func (NoopApplicationMetadataService) CreateOrUpdate(context.Context, appmetadata.ApplicationMetadata) error {
	return nil
}

func (NoopApplicationMetadataService) List(context.Context, appmetadata.ApplicationMetadata) (apps []appmetadata.ApplicationMetadata, err error) {
	return apps, err
}

func (NoopApplicationMetadataService) Get(context.Context, string) (app appmetadata.ApplicationMetadata, err error) {
	return app, err
}

func (NoopApplicationMetadataService) Delete(context.Context, string) error {
	return nil
}
