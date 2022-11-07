package storage

import "context"

// NoopApplicationMetadataService implements same methods as ApplicationMetadataService
// But it doesn't do anything when called
type NoopApplicationMetadataService struct{}

func (NoopApplicationMetadataService) CreateOrUpdate(context.Context, ApplicationMetadata) error {
	return nil
}

func (NoopApplicationMetadataService) List(context.Context, ApplicationMetadata) (apps []ApplicationMetadata, err error) {
	return apps, err
}

func (NoopApplicationMetadataService) Get(context.Context, string) (app ApplicationMetadata, err error) {
	return app, err
}

func (NoopApplicationMetadataService) Delete(context.Context, string) error {
	return nil
}
