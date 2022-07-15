package admin

import (
	"context"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

type AdminService struct {
	storage Storage
}

type Storage interface {
	storage.AppGetter
	storage.AppDeleter
}

func NewService(v Storage) *AdminService {
	m := &AdminService{
		v,
	}
	return m
}

func (m *AdminService) GetAppNames(ctx context.Context) (appNames []string) {
	return m.storage.GetAppNames(ctx)
}

func (m *AdminService) GetApps(ctx context.Context) storage.GetAppsOutput {
	return m.storage.GetApps(ctx)
}

func (m *AdminService) DeleteApp(appname string) error {
	return m.storage.DeleteApp(context.TODO(), appname)
}
