package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/pyroscope-io/pyroscope/pkg/service"
	"github.com/pyroscope-io/pyroscope/pkg/sqlstore"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exporter"
	"github.com/pyroscope-io/pyroscope/pkg/health"
	"github.com/pyroscope-io/pyroscope/pkg/parser"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("server", func() {
	type testServices struct {
		s *storage.Storage
		k service.APIKeyService
	}
	ingest := func(lines string, app string) {
		u, _ := url.Parse("http://localhost:4040/ingest")
		q := u.Query()
		q.Add("name", app)
		q.Add("from", strconv.Itoa(int(testing.ParseTime("2020-01-01-01:01:00").Unix())))
		q.Add("until", strconv.Itoa(int(testing.ParseTime("2020-01-01-01:01:10").Unix())))
		q.Add("format", "lines")
		u.RawQuery = q.Encode()
		req, err := http.NewRequest("POST", u.String(), bytes.NewBuffer([]byte(lines)))
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Content-Type", "text/plain")
		res, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		Expect(res.StatusCode).To(Equal(200))
	}
	deleteApp := func(app string, key string) *http.Response {
		input := struct {
			Name string `json:"name"`
		}{app}
		body, _ := json.Marshal(input)
		req, err := http.NewRequest("DELETE", "http://localhost:4040/api/apps", bytes.NewBuffer(body))
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		Expect(err).ToNot(HaveOccurred())
		res, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		return res
	}
	runServer := func(cfg **config.Config, cb func(s testServices)) {
		defer GinkgoRecover()
		s, err := storage.New(
			storage.NewConfig(&(*cfg).Server),
			logrus.StandardLogger(),
			prometheus.NewRegistry(),
			new(health.Controller),
			storage.NoopApplicationMetadataService{},
		)
		Expect(err).ToNot(HaveOccurred())
		e, _ := exporter.NewExporter(nil, nil)
		sql, err := sqlstore.Open(&(*cfg).Server)
		Expect(err).ToNot(HaveOccurred())

		l := logrus.New()

		c, _ := New(Config{
			Configuration:           &(*cfg).Server,
			Storage:                 s,
			Ingester:                parser.New(logrus.StandardLogger(), s, e),
			Logger:                  l,
			MetricsRegisterer:       prometheus.NewRegistry(),
			ExportedMetricsRegistry: prometheus.NewRegistry(),
			Notifier:                mockNotifier{},
			DB:                      sql.DB(),
		})
		startController(c, "http", ":4040")
		defer c.Stop()

		k := service.NewAPIKeyService(sql.DB())
		cb(testServices{s, k})
	}
	createToken := func(s testServices, role model.Role) string {
		params := model.CreateAPIKeyParams{Name: "t" + strconv.Itoa(int(role)), Role: role}
		_, key, err := s.k.CreateAPIKey(context.TODO(), params)
		Expect(err).ToNot(HaveOccurred())
		return key
	}
	checkRoleAccess := func(s testServices, role model.Role, expectAccess bool, expectStatus int) {
		ingest("foo;bar\nfoo;bar\nfoo;baz\nfoo;baz\nfoo;baz\n", "test.app1")
		ingest("foo;bar\nfoo;bar\nfoo;baz\nfoo;baz\nfoo;baz\n", "test.app2")
		time.Sleep(100 * time.Millisecond)
		Expect(s.s.GetAppNames(context.TODO())).To(Equal([]string{"test.app1", "test.app2"}))
		key := ""
		if role != model.InvalidRole {
			key = createToken(s, role)
		}
		res := deleteApp("test.app2", key)
		Expect(res.StatusCode).To(Equal(expectStatus))
		if expectAccess {
			Expect(s.s.GetAppNames(context.TODO())).To(Equal([]string{"test.app1"}))
		} else {
			Expect(s.s.GetAppNames(context.TODO())).To(Equal([]string{"test.app1", "test.app2"}))
		}
	}

	testing.WithConfig(func(cfg **config.Config) {
		Describe("http api admin controller", func() {
			Context("with Auth enabled", func() {
				It("should be accessible to only admin token", func() {
					(*cfg).Server.Auth.Internal.Enabled = true
					(*cfg).Server.EnableExperimentalAdmin = true
					runServer(cfg, func(s testServices) {
						checkRoleAccess(s, model.AdminRole, true, http.StatusOK)
						checkRoleAccess(s, model.AgentRole, false, http.StatusForbidden)
						checkRoleAccess(s, model.ReadOnlyRole, false, http.StatusForbidden)
						checkRoleAccess(s, model.InvalidRole, false, http.StatusUnauthorized)
					})
				})
			})
			Context("with Auth disabled", func() {
				// notice that the routes' access is granular
				// FIXME update the tests to test each route individually
				It("should never be accessible", func() {
					(*cfg).Server.Auth.Internal.Enabled = false
					(*cfg).Server.EnableExperimentalAdmin = true
					runServer(cfg, func(s testServices) {
						checkRoleAccess(s, model.AdminRole, false, http.StatusForbidden)
						checkRoleAccess(s, model.AgentRole, false, http.StatusForbidden)
						checkRoleAccess(s, model.ReadOnlyRole, false, http.StatusForbidden)
						checkRoleAccess(s, model.InvalidRole, false, http.StatusForbidden)
					})
				})
			})
		})
	})
})
