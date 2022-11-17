package service

import (
	"context"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"golang.org/x/sync/errgroup"
)

type AppDeleter interface {
	DeleteApp(ctx context.Context, appName string) error
}

type ApplicationService struct {
	appMetadataSvc ApplicationMetadataService
	storageDeleter AppDeleter
}

// NewApplicationService creates an ApplicationService
// Which just delegates to its underlying ApplicationMetadataService
// Except when deleting, which is then forward to both ApplicationMetadataService and storageDeleter
func NewApplicationService(appMetadataSvc ApplicationMetadataService, storageDeleter AppDeleter) ApplicationService {
	return ApplicationService{
		appMetadataSvc: appMetadataSvc,
		storageDeleter: storageDeleter,
	}
}

// List delegates to ApplicationMetadataService
func (svc ApplicationService) List(ctx context.Context) (apps []storage.ApplicationMetadata, err error) {
	return svc.appMetadataSvc.List(ctx)
}

// Get delegates to ApplicationMetadataService
func (svc ApplicationService) Get(ctx context.Context, name string) (storage.ApplicationMetadata, error) {
	return svc.appMetadataSvc.Get(ctx, name)
}

// CreateOrUpdate delegates to ApplicationMetadataService
// For data ingestion, look for storage
func (svc ApplicationService) CreateOrUpdate(ctx context.Context, application storage.ApplicationMetadata) error {
	return svc.appMetadataSvc.CreateOrUpdate(ctx, application)
}

// Delete deletes apps from both storage and ApplicationMetadata
func (svc ApplicationService) Delete(ctx context.Context, name string) error {
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return svc.storageDeleter.DeleteApp(ctx, name)
	})

	g.Go(func() error {
		return svc.appMetadataSvc.DeleteApp(ctx, name)
	})

	return g.Wait()
}

// GetAppNames fetches all applications and returns only its names
func (svc ApplicationService) GetAppNames(ctx context.Context) ([]string, error) {
	apps, err := svc.List(ctx)
	if err != nil {
		return []string{}, err
	}

	appNames := make([]string, len(apps))
	for i, appName := range apps {
		appNames[i] = appName.FQName
	}

	return appNames, nil
}
