package service

import (
	"context"
	"errors"

	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"gorm.io/gorm"
)

type ApplicationService struct {
	db *gorm.DB
}

func NewApplicationService(db *gorm.DB) ApplicationService {
	return ApplicationService{db: db}
}

func (svc ApplicationService) List(ctx context.Context) (apps []storage.Application, err error) {
	tx := svc.db.WithContext(ctx)
	result := tx.Find(&apps)
	return apps, svc.handleError(result.Error)
}

func (svc ApplicationService) Get(ctx context.Context, name string) (storage.Application, error) {
	app := storage.Application{}
	if err := model.ValidateAppName(name); err != nil {
		return app, err
	}

	tx := svc.db.WithContext(ctx)
	res := tx.Where("name = ?", name).First(&app)
	return app, svc.handleError(res.Error)
}

func (svc ApplicationService) CreateOrUpdate(ctx context.Context, application storage.Application) error {
	if err := model.ValidateAppName(application.FullyQualifiedName); err != nil {
		return err
	}

	tx := svc.db.WithContext(ctx)

	// Only update the field if it's populated
	err := tx.Where(storage.Application{
		FullyQualifiedName: application.FullyQualifiedName,
	}).Assign(application).FirstOrCreate(&storage.Application{}).Error
	return svc.handleError(err)
}

func (svc ApplicationService) Delete(ctx context.Context, name string) error {
	if err := model.ValidateAppName(name); err != nil {
		return err
	}

	tx := svc.db.WithContext(ctx)
	err := tx.Where("name = ?", name).Delete(storage.Application{}).Error
	return svc.handleError(err)
}

func (ApplicationService) handleError(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return model.ErrApplicationNotFound
	default:
		return err
	}
}
