package admin

type AdminService struct {
	AppNamesGetter
}

type AppNamesGetter interface {
	GetAppNames() []string
}

func NewService(v AppNamesGetter) *AdminService {
	m := &AdminService{
		v,
	}

	return m
}

func (m *AdminService) GetApps() (appNames []string) {
	return m.GetAppNames()
}
