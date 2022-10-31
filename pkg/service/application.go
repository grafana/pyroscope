package service

import (
	"context"
	"fmt"

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
	//tx := svc.db.WithContext(ctx)

	result := svc.db.Find(&apps)
	return apps, result.Error
	//	return tx.Fin(&application).Error
	//	return []storage.Application{}, nil
	//TODO implement me
	//panic("implement me")
}

func (ApplicationService) Get(ctx context.Context, name string) (storage.Application, error) {
	//TODO implement me
	//panic("implement me")
	return storage.Application{}, nil
}

func (svc ApplicationService) CreateOrUpdate(ctx context.Context, application storage.Application) error {
	fmt.Println("creating or updating", application)
	tx := svc.db.WithContext(ctx)
	return tx.Create(&application).Error

	return nil
	//TODO implement me
	//	panic("implement me")
}

func (ApplicationService) Delete(ctx context.Context, name string) error {
	//TODO implement me
	//panic("implement me")
	return nil
}
