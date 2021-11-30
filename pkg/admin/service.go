package admin

type AdminService struct {
	storage Storage
}

type Storage interface {
	GetAppNames() []string
	DeleteApp(appname string) error
}

func NewService(v Storage) *AdminService {
	m := &AdminService{
		v,
	}

	return m
}

func (m *AdminService) GetApps() (appNames []string) {
	return m.storage.GetAppNames()
}

func (m *AdminService) DeleteApp(appname string) error {
	return m.storage.DeleteApp(appname)
}
