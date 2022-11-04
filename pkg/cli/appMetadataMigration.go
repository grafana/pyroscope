package cli

import (
	"context"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

type AppNamesGetter interface {
	GetAppNames(ctx context.Context) []string
}

type AppMetadataSaver interface {
	CreateOrUpdate(_ context.Context, _ storage.Application) error
	List(context.Context) ([]storage.Application, error)
}

type AppMetadataMigrator struct {
	appNamesGetter   AppNamesGetter
	appMetadataSaver AppMetadataSaver
}

func NewAppMetadataMigrator(appNamesGetter AppNamesGetter, appMetadataSaver AppMetadataSaver) *AppMetadataMigrator {
	return &AppMetadataMigrator{
		appNamesGetter:   appNamesGetter,
		appMetadataSaver: appMetadataSaver,
	}
}

// Migrate creates Applications given a list of app names
func (m *AppMetadataMigrator) Migrate() error {
	ctx := context.Background()

	// Get all app names
	appNamesFromOrigin := m.appNamesGetter.GetAppNames(ctx)
	apps, err := m.appMetadataSaver.List(ctx)
	if err != nil {
		return err
	}

	// TODO skip if not necessary

	// Convert slice -> map
	appMap := make(map[string]storage.Application)
	for _, a := range apps {
		appMap[a.Name] = a
	}

	// If they don't exist already
	for _, a := range appNamesFromOrigin {
		if _, ok := appMap[a]; !ok {
			m.appMetadataSaver.CreateOrUpdate(ctx, storage.Application{
				Name: a,
			})
		}
	}

	// Write to MetadataSaver
	return nil
}
