package service

import (
	"context"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

type AppDeleter interface {
	DeleteApp(ctx context.Context, appName string) error
}

type ApplicationService struct {
	ApplicationMetadataService
	storageDeleter AppDeleter
}

// NewApplicationService creates an ApplicationService
// Which just delegates to its underlying ApplicationMetadataService
// Except when deleting, which is then forward to both ApplicationMetadataService and storageDeleter
func NewApplicationService(appMetadataSvc ApplicationMetadataService, storageDeleter AppDeleter) ApplicationService {
	return ApplicationService{
		appMetadataSvc,
		storageDeleter,
	}
}

// Delete deletes apps from both storage and ApplicationMetadata
// It first deletes from storage and only then deletes its metadata
func (svc ApplicationService) Delete(ctx context.Context, name string) error {
	err := model.ValidateAppName(name)
	if err != nil {
		return err
	}

	err = svc.storageDeleter.DeleteApp(ctx, name)
	if err != nil {
		return err
	}

	return svc.ApplicationMetadataService.Delete(ctx, name)
}
