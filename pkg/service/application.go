package service

import (
	"context"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ApplicationService struct {
	db *gorm.DB
}

func NewApplicationService(db *gorm.DB) ApplicationService {
	return ApplicationService{db: db}
}

func (svc ApplicationService) List(ctx context.Context) (apps []storage.Application, err error) {
	result := svc.db.Find(&apps)
	return apps, result.Error
}

func (svc ApplicationService) Get(ctx context.Context, name string) (storage.Application, error) {
	app := storage.Application{}
	res := svc.db.Where("name = ?", name).First(&app)
	return app, res.Error
}

func (svc ApplicationService) CreateOrUpdate(ctx context.Context, application storage.Application) error {
	tx := svc.db.WithContext(ctx)
	return tx.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Create(&application).Error
}

func (svc ApplicationService) Delete(ctx context.Context, name string) error {
	return svc.db.Where("name = ?", name).Delete(storage.Application{}).Error
}
