package cli

import (
	"context"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/sirupsen/logrus"
)

type AppNamesGetter interface {
	GetAppNames(ctx context.Context) []string
}

type AppMetadataSaver interface {
	CreateOrUpdate(_ context.Context, _ storage.Application) error
	List(context.Context) ([]storage.Application, error)
}

type AppMetadataMigrator struct {
	logger           *logrus.Logger
	appNamesGetter   AppNamesGetter
	appMetadataSaver AppMetadataSaver
}

func NewAppMetadataMigrator(logger *logrus.Logger, appNamesGetter AppNamesGetter, appMetadataSaver AppMetadataSaver) *AppMetadataMigrator {
	return &AppMetadataMigrator{
		logger:           logger,
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
		appMap[a.FullyQualifiedName] = a
	}

	// If they don't exist already
	for _, a := range appNamesFromOrigin {
		if _, ok := appMap[a]; !ok {
			logrus.Info("Migrating app: ", a)
			// Write to MetadataSaver
			m.appMetadataSaver.CreateOrUpdate(ctx, storage.Application{
				FullyQualifiedName: a,
			})
		}
	}

	return nil
}
