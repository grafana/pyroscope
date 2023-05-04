package chstore

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"time"

	chgorm "gorm.io/driver/clickhouse"

	"github.com/ClickHouse/clickhouse-go/v2"
	"gorm.io/gorm"

	// "github.com/pyroscope-io/pyroscope/pkg/chstore/migrations"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	// "gorm.io/gorm"
)

type CHStore struct {
	config *config.Server

	db  *clickhouse.Conn
	orm *gorm.DB
}

// type ClickHouseDialector struct {
// 	conn *clickhouse.Conn
// }

// func (d ClickHouseDialector) Name() string {
// 	return "clickhouse"
// }

// func (d ClickHouseDialector) Initialize(db *gorm.DB) error {
// 	return nil
// }

// func (d ClickHouseDialector) Connect(_ context.Context) (gorm.Connector, error) {
// 	return d, nil
// }

// func (d ClickHouseDialector) DriverName() string {
// 	return "clickhouse"
// }

// func (d ClickHouseDialector) DSN() string {
// 	return ""
// }

// func (d ClickHouseDialector) ConnectWithDSN(dsn string) (gorm.SQLConn, error) {
// 	return nil, nil
// }

// func (d ClickHouseDialector) Ping(ctx context.Context) error {
// 	return d.conn.Ping()
// }

// func (d ClickHouseDialector) Dialect() gorm.Dialector {
// 	return d
// }

// func (d ClickHouseDialector) BindVarTo(writer gorm.GormWriter, stmt *gorm.Statement, v interface{}) {
// 	// Implement BindVarTo if necessary
// }

// func NewClickHouseDialector(conn *clickhouse.Conn) ClickHouseDialector {
// 	return ClickHouseDialector{conn: conn}
// }

func Open(c *config.Server) (*CHStore, error) {
	s := CHStore{config: c}

	dsn := "clickhouse://clickhouse:clickhouse@localhost:9000/default"

	dsnURL, err := url.Parse(dsn)
	if err != nil {
		log.Fatalf("could not connect to clickhouse: %s", err)
		return nil, err
	}

	options := &clickhouse.Options{
		Addr: []string{dsnURL.Host},
	}
	if dsnURL.Query().Get("username") != "" {
		auth := clickhouse.Auth{
			Username: dsnURL.Query().Get("username"),
			Password: dsnURL.Query().Get("password"),
		}
		options.Auth = auth
	}
	options.MaxOpenConns = 180
	options.MaxIdleConns = 180
	options.DialTimeout = time.Second * 30

	conn, err := clickhouse.Open(options)

	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}
	s.db = &conn

	if err = s.Ping(context.Background()); err != nil {
		return nil, err
	}

	// dialector := NewClickHouseDialector(conn)

	s.orm, err = gorm.Open(chgorm.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// if err = migrations.Migrate(conn, c); err != nil {
	// 	return nil, err
	// }

	return &s, nil
}

func (s *CHStore) DB() *gorm.DB { return s.orm }

func (s *CHStore) Close() error { return (*s.db).Close() }

func (s *CHStore) Ping(ctx context.Context) error {
	return (*s.db).Ping(ctx)
}
