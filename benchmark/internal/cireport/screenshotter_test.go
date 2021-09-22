package cireport_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/benchmark/internal/cireport"
)

var _ = Describe("screenshotter", func() {

	Context("AllPanels", func() {
		var ts *httptest.Server
		var s cireport.GrafanaScreenshotter
		var ctx context.Context

		BeforeEach(func() {
			ts = createFakeGrafanaServer()
			ctx = context.Background()
			s = cireport.GrafanaScreenshotter{
				GrafanaURL:     ts.URL,
				TimeoutSeconds: 10,
			}
		})

		AfterEach(func() {
			ts.Close()
		})

		It("should handle dashboards with no rows", func() {
			panels, err := s.AllPanels(ctx, "dashboard-without-rows", 0, 0)

			Expect(err).NotTo(HaveOccurred())
			Expect(panels).To(HaveLen(2))
		})

		It("should handle dashboards with rows", func() {
			panels, err := s.AllPanels(ctx, "dashboard-with-rows", 0, 0)

			Expect(err).NotTo(HaveOccurred())
			Expect(panels).To(HaveLen(3))
		})

		It("should handle dashboards panels nested into rows", func() {
			panels, err := s.AllPanels(ctx, "dashboard-with-panels-nested-into-rows", 0, 0)

			Expect(err).NotTo(HaveOccurred())
			Expect(panels).To(HaveLen(4))
		})
	})
})

func createFakeGrafanaServer() *httptest.Server {
	const DashWithoutRows = `
{
	"dashboard": {
		"panels": [
			{ "id": 0, "title": "my-title" },
			{ "id": 0, "title": "my-title" }
		]
	}
}`
	const DashWithRows = `
{
	"dashboard": {
		"panels": [
			{ "id": 0, "title": "my-title" },
			{ "id": 0, "title": "my-title", "type": "row" },
			{ "id": 0, "title": "my-title" },
			{ "id": 0, "title": "my-title", "type": "row" },
			{ "id": 0, "title": "my-title" }
		]
	}
}`

	const DashWithPanelsNestedIntoRow = `
{
	"dashboard": {
		"rows": [
		{
			"panels": [
				{ "id": 0, "title": "my-title" },
				{ "id": 0, "title": "my-title" },
				{ "id": 0, "title": "my-title" }
			]
		},
		{
			"panels": [
				{ "id": 0, "title": "my-title" }
			]
		}]
	}
}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/dashboards/uid/dashboard-without-rows":
			fmt.Fprintln(w, DashWithoutRows)

		case "/api/dashboards/uid/dashboard-with-rows":
			fmt.Fprintln(w, DashWithRows)

		case "/api/dashboards/uid/dashboard-with-panels-nested-into-rows":
			fmt.Fprintln(w, DashWithPanelsNestedIntoRow)

		case "/render/d-solo/dashboard-without-rows":
			fmt.Fprintln(w, ``)

		case "/render/d-solo/dashboard-with-rows":
			fmt.Fprintln(w, ``)

		case "/render/d-solo/dashboard-with-panels-nested-into-rows":
			fmt.Fprintln(w, ``)

		default:
			panic("invalid url " + r.URL.String())
		}
	}))

	return ts
}
