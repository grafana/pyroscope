package storage

// import (
// 	"context"
// 	"database/sql"
// 	"fmt"
// 	"strings"
// 	"sync"
// 	"time"

// 	"github.com/ClickHouse/clickhouse-go/v2"
// 	"github.com/prometheus/client_golang/prometheus"
// 	// "github.com/prometheus/common/model"
// 	"github.com/pyroscope-io/pyroscope/pkg/health"
// 	// "github.com/pyroscope-io/pyroscope/pkg/model/appmetadata"
// 	"github.com/pyroscope-io/pyroscope/pkg/storage/cache"
// 	"github.com/pyroscope-io/pyroscope/pkg/storage/labels"
// 	// "github.com/pyroscope-io/pyroscope/pkg/storage/segment"
// 	// "github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
// 	"github.com/sirupsen/logrus"
// )

// type chStorage2 struct {
// 	config     *Config
// 	logger     *logrus.Entry
// 	metrics    *metrics
// 	cacheTTL   time.Duration
// 	gcSizeDiff int64
// }

// type ChStorage struct {
// 	config *Config
// 	*storageOptions

// 	logger *logrus.Logger
// 	*metrics

// 	db         *sql.DB
// 	labels     *labels.Labels
// 	exemplars  *exemplars
// 	appSvc     ApplicationMetadataSaver
// 	hc         *health.Controller
// 	tasksMutex sync.Mutex
// 	tasksWG    sync.WaitGroup
// 	stop       chan struct{}
// 	putMutex   sync.Mutex
// }

// func (s *ChStorage) Close() error {
// 	// Stop all periodic and maintenance tasks.
// 	close(s.stop)
// 	s.logger.Debug("waiting for storage tasks to finish")
// 	s.tasksWG.Wait()
// 	s.logger.Debug("storage tasks finished")

// 	// Close database connection.
// 	if err := s.db.Close(); err != nil {
// 		return err
// 	}

// 	return nil
// }

// func (s *ChStorage) Write(_ context.Context, req *WriteRequest) error {
// 	s.putMutex.Lock()
// 	defer s.putMutex.Unlock()

// 	// Prepare the INSERT query
// 	query := fmt.Sprintf(
// 		"INSERT INTO %s (timestamp, value) VALUES ", "main")

// 	var values []string
// 	for _, ts := range req.Timeseries {
// 		for _, s := range ts.Samples {
// 			// Add a new row to the query for each sample
// 			values = append(values, fmt.Sprintf("(%d, %f)", s.Timestamp.Unix(), s.Value))
// 		}
// 	}

// 	// Combine all the rows into a single query string
// 	query += strings.Join(values, ",")

// 	// Execute the query
// 	_, err := s.db.Exec(query)
// 	if err != nil {
// 		return err
// 	}

// 	return nil
// }

// // ------------------------------------------------
// type chdb struct {
// 	name    string
// 	DB      *clickhouse.Conn
// 	logger  *logrus.Entry
// 	gcCount prometheus.Counter
// }

// type ClickhouseDBWithCache interface {
// 	DB() *clickhouse.Conn
// 	Cache() *cache.Cache
// 	Name() string
// 	Close() error
// }

// func (s *Storage) newClickHouse(name string, p Prefix, codec cache.Codec) (ClickhouseDBWithCache, error) {
// 	var d *db
// 	var err error
// 	logger := logrus.New()
// 	logger.SetLevel(logrus.DebugLevel)

// 	// dsn := fmt.Sprintf("clickhouse://clickhouse:clickhouse@localhost:9000/default")

// 	conn, err := clickhouse.Open(&clickhouse.Options{
// 		Addr: []string{"127.0.0.1:9000"},
// 		Auth: clickhouse.Auth{
// 			Database: "default",
// 			Username: "clickhouse",
// 			Password: "clickhouse",
// 		},
// 	})
// 	if err != nil {
// 		return nil, err
// 	}

// 	d = &db{
// 		name:   name,
// 		CHDB:   &conn,
// 		logger: s.logger.WithField("db", name),
// 	}

// 	if codec != nil {
// 		d.Cache = cache.New(cache.Config{
// 			CHDB:    &conn,
// 			Metrics: s.metrics.createCacheMetrics(name),
// 			TTL:     s.cacheTTL,
// 			Prefix:  p.String(),
// 			Codec:   codec,
// 		})
// 	}

// 	// s.maintenanceTask(s.clickhouseGCTaskInterval, func() {
// 	// 	// TODO: implement garbage collection for ClickHouse
// 	// })

// 	return d, nil
// }
