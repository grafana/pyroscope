package service

import (
	"context"

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
	tx := svc.db.WithContext(ctx)
	app := storage.Application{}
	res := tx.Where("name = ?", name).First(&app)
	return app, res.Error
}

func (svc ApplicationService) CreateOrUpdate(ctx context.Context, application storage.Application) error {
	tx := svc.db.WithContext(ctx)

	// Only update empty values
	return tx.Where(storage.Application{
		Name: application.Name,
	}).Assign(application).FirstOrCreate(&storage.Application{}).Error
}

func (svc ApplicationService) Delete(ctx context.Context, name string) error {
	tx := svc.db.WithContext(ctx)
	return tx.Where("name = ?", name).Delete(storage.Application{}).Error
}
