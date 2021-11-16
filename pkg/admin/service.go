package admin

import (
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

type AdminService struct {
	Storage
}

type Storage interface {
	GetAppNames() []string
	Delete(di *storage.DeleteInput) error
}

func NewService(v Storage) *AdminService {
	m := &AdminService{
		v,
	}

	return m
}

func (m *AdminService) GetApps() (appNames []string) {
	return m.GetAppNames()
}

func (m *AdminService) DeleteApp(appname string) error {
	key, err := segment.ParseKey(appname)
	if err != nil {
		return err
	}

	return m.Delete(&storage.DeleteInput{
		Key: key,
	})
}
