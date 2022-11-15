package service

import (
	"context"
	"errors"

	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"gorm.io/gorm"
)

type ApplicationMetadataService struct {
	db *gorm.DB
}

func NewApplicationMetadataService(db *gorm.DB) ApplicationMetadataService {
	return ApplicationMetadataService{db: db}
}

func (svc ApplicationMetadataService) List(ctx context.Context) (apps []storage.ApplicationMetadata, err error) {
	tx := svc.db.WithContext(ctx)
	result := tx.Find(&apps)
	return apps, result.Error
}

func (svc ApplicationMetadataService) Get(ctx context.Context, name string) (storage.ApplicationMetadata, error) {
	app := storage.ApplicationMetadata{}
	if err := model.ValidateAppName(name); err != nil {
		return app, err
	}

	tx := svc.db.WithContext(ctx)
	res := tx.Where("fq_name = ?", name).First(&app)

	switch {
	case errors.Is(res.Error, gorm.ErrRecordNotFound):
		return app, model.ErrApplicationNotFound
	default:
		return app, res.Error
	}
}

func (svc ApplicationMetadataService) CreateOrUpdate(ctx context.Context, application storage.ApplicationMetadata) error {
	if err := model.ValidateAppName(application.FQName); err != nil {
		return err
	}

	tx := svc.db.WithContext(ctx)

	// Only update the field if it's populated
	return tx.Where(storage.ApplicationMetadata{
		FQName: application.FQName,
	}).Assign(application).FirstOrCreate(&storage.ApplicationMetadata{}).Error
}

func (svc ApplicationMetadataService) Delete(ctx context.Context, name string) error {
	if err := model.ValidateAppName(name); err != nil {
		return err
	}

	tx := svc.db.WithContext(ctx)
	return tx.Where("fq_name = ?", name).Delete(storage.ApplicationMetadata{}).Error
}
