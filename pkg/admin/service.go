package admin

import "context"

type AdminService struct {
	storage Storage
}

type Storage interface {
	GetAppNames(context.Context) []string
	DeleteApp(ctx context.Context, appname string) error
}

func NewService(v Storage) *AdminService {
	m := &AdminService{
		v,
	}

	return m
}

func (m *AdminService) GetApps() (appNames []string) {
	return m.storage.GetAppNames(context.TODO())
}

func (m *AdminService) DeleteApp(appname string) error {
	return m.storage.DeleteApp(context.TODO(), appname)
}
