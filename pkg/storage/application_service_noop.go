package storage

import "context"

// NoopApplicationService implements same methods as ApplicationService
// But it doesn't do anything when called
type NoopApplicationService struct{}

func (NoopApplicationService) CreateOrUpdate(context.Context, Application) error {
	return nil
}

func (NoopApplicationService) List(context.Context, Application) (apps []Application, err error) {
	return apps, err
}

func (NoopApplicationService) Get(context.Context, string) (app Application, err error) {
	return app, err
}

func (NoopApplicationService) Delete(context.Context, string) error {
	return nil
}
