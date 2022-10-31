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

func (ApplicationService) List(ctx context.Context) ([]storage.Application, error) {
	return []storage.Application{}, nil
	//TODO implement me
	//panic("implement me")
}

func (ApplicationService) Get(ctx context.Context, name string) (storage.Application, error) {
	//TODO implement me
	//panic("implement me")
	return storage.Application{}, nil
}

func (svc ApplicationService) CreateOrUpdate(ctx context.Context, application storage.Application) error {
	fmt.Println("creating or updating")
	tx := svc.db.WithContext(ctx)
	return tx.Create(application).Error

	return nil
	//TODO implement me
	//	panic("implement me")
}

func (ApplicationService) Delete(ctx context.Context, name string) error {
	//TODO implement me
	//panic("implement me")
	return nil
}
