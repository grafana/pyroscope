package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/sqlstore/migrations"
)

type SQLStore struct {
	config config.Database

	db  *sql.DB
	orm *gorm.DB
}

func Open(c *config.Server) (*SQLStore, error) {
	s := SQLStore{config: c.Database}
	var err error
	switch s.config.Type {
	case "sqlite3":
		err = s.openSQLiteDB()
	default:
		return nil, errors.New("unknown db type")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}
	if err = s.Ping(context.Background()); err != nil {
		return nil, err
	}
	if err = migrations.Migrate(s.orm, c); err != nil {
		return nil, err
	}
	return &s, nil
}

func (s *SQLStore) DB() *gorm.DB { return s.orm }

func (s *SQLStore) Close() error { return s.db.Close() }

func (s *SQLStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *SQLStore) openSQLiteDB() (err error) {
	s.orm, err = gorm.Open(sqlite.Open(s.config.URL), &gorm.Config{
		Logger: logger.Discard,
	})
	s.db, err = s.orm.DB()
	return err
}
