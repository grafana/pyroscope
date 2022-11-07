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
	return apps, result.Error
}

func (svc ApplicationService) Get(ctx context.Context, name string) (storage.Application, error) {
	app := storage.Application{}
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

func (svc ApplicationService) CreateOrUpdate(ctx context.Context, application storage.Application) error {
	if err := model.ValidateAppName(application.FQName); err != nil {
		return err
	}

	tx := svc.db.WithContext(ctx)

	// Only update the field if it's populated
	return tx.Where(storage.Application{
		FQName: application.FQName,
	}).Assign(application).FirstOrCreate(&storage.Application{}).Error
}

func (svc ApplicationService) Delete(ctx context.Context, name string) error {
	if err := model.ValidateAppName(name); err != nil {
		return err
	}

	tx := svc.db.WithContext(ctx)
	return tx.Where("fq_name = ?", name).Delete(storage.Application{}).Error
}
