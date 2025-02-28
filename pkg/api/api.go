// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/api/api.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package api

import (
	"context"
	"flag"
	"fmt"

	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/felixge/fgprof"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gorilla/mux"
	"github.com/grafana/dskit/kv/memberlist"
	"github.com/grafana/dskit/middleware"
	"github.com/grafana/dskit/server"
	grpcgw "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	"github.com/grafana/pyroscope/public"

	"github.com/grafana/pyroscope/api/gen/proto/go/adhocprofiles/v1/adhocprofilesv1connect"
	"github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1/ingesterv1connect"
	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	"github.com/grafana/pyroscope/api/gen/proto/go/settings/v1/settingsv1connect"
	statusv1 "github.com/grafana/pyroscope/api/gen/proto/go/status/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/storegateway/v1/storegatewayv1connect"
	"github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1/vcsv1connect"
	"github.com/grafana/pyroscope/api/gen/proto/go/version/v1/versionv1connect"
	"github.com/grafana/pyroscope/api/openapiv2"
	"github.com/grafana/pyroscope/pkg/adhocprofiles"
	connectapi "github.com/grafana/pyroscope/pkg/api/connect"
	"github.com/grafana/pyroscope/pkg/compactor"
	"github.com/grafana/pyroscope/pkg/distributor"
	"github.com/grafana/pyroscope/pkg/frontend"
	"github.com/grafana/pyroscope/pkg/frontend/frontendpb/frontendpbconnect"
	"github.com/grafana/pyroscope/pkg/ingester"
	"github.com/grafana/pyroscope/pkg/ingester/otlp"
	"github.com/grafana/pyroscope/pkg/ingester/pyroscope"
	"github.com/grafana/pyroscope/pkg/operations"
	"github.com/grafana/pyroscope/pkg/querier"
	"github.com/grafana/pyroscope/pkg/scheduler"
	"github.com/grafana/pyroscope/pkg/scheduler/schedulerpb/schedulerpbconnect"
	"github.com/grafana/pyroscope/pkg/settings"
	"github.com/grafana/pyroscope/pkg/storegateway"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/util/gziphandler"
	"github.com/grafana/pyroscope/pkg/validation/exporter"
)

type Config struct {
	// The following configs are injected by the upstream caller.
	HTTPAuthMiddleware middleware.Interface `yaml:"-"`
	GrpcAuthMiddleware connect.Option       `yaml:"-"`
	BaseURL            string               `yaml:"base-url"`
}

type API struct {
	server             *server.Server
	httpAuthMiddleware middleware.Interface
	grpcGatewayMux     *grpcgw.ServeMux
	grpcAuthMiddleware connect.Option
	grpcLogMiddleware  connect.Option
	recoveryMiddleware connect.Option

	cfg       Config
	logger    log.Logger
	indexPage *IndexPageContent
}

func New(cfg Config, s *server.Server, grpcGatewayMux *grpcgw.ServeMux, logger log.Logger) (*API, error) {
	api := &API{
		cfg:                cfg,
		httpAuthMiddleware: cfg.HTTPAuthMiddleware,
		server:             s,
		logger:             logger,
		indexPage:          NewIndexPageContent(),
		grpcGatewayMux:     grpcGatewayMux,
		grpcAuthMiddleware: cfg.GrpcAuthMiddleware,
		grpcLogMiddleware:  connect.WithInterceptors(util.NewLogInterceptor(logger)),
		recoveryMiddleware: connect.WithInterceptors(util.RecoveryInterceptor),
	}

	// If no authentication middleware is present in the config, use the default authentication middleware.
	if cfg.HTTPAuthMiddleware == nil {
		api.httpAuthMiddleware = middleware.AuthenticateUser
	}

	return api, nil
}

// RegisterRoute registers a single route enforcing HTTP methods. A single
// route is expected to be specific about which HTTP methods are supported.
func (a *API) RegisterRoute(path string, handler http.Handler, auth, gzipEnabled bool, method string, methods ...string) {
	methods = append([]string{method}, methods...)
	level.Debug(a.logger).
		Log("msg", "api: registering route", "methods", strings.Join(methods, ","), "path", path, "auth", auth, "gzip", gzipEnabled)
	a.newRoute(path, handler, false, auth, gzipEnabled, methods...)
}

func (a *API) RegisterRoutesWithPrefix(prefix string, handler http.Handler, auth, gzipEnabled bool, methods ...string) {
	level.Debug(a.logger).
		Log("msg", "api: registering route", "methods", strings.Join(methods, ","), "prefix", prefix, "auth", auth, "gzip", gzipEnabled)
	a.newRoute(prefix, handler, true, auth, gzipEnabled, methods...)
}

//nolint:unparam
func (a *API) newRoute(path string, handler http.Handler, isPrefix, auth, gzip bool, methods ...string) (route *mux.Route) {
	if auth {
		handler = a.httpAuthMiddleware.Wrap(handler)
	}
	if gzip {
		handler = gziphandler.GzipHandler(handler)
	}
	if isPrefix {
		route = a.server.HTTP.PathPrefix(path)
	} else {
		route = a.server.HTTP.Path(path)
	}
	if len(methods) > 0 {
		route = route.Methods(methods...)
	}
	route = route.Handler(handler)

	return route
}

// RegisterAPI registers the standard endpoints associated with a running Pyroscope.
func (a *API) RegisterAPI(statusService statusv1.StatusServiceServer) error {
	// register admin page
	a.RegisterRoute("/admin", indexHandler("", a.indexPage), false, true, "GET")
	// expose openapiv2 definition
	openapiv2Handler, err := openapiv2.Handler()
	if err != nil {
		return fmt.Errorf("unable to initialize openapiv2 handler: %w", err)
	}
	a.RegisterRoute("/api/swagger.json", openapiv2Handler, false, true, "GET")
	a.indexPage.AddLinks(openAPIDefinitionWeight, "OpenAPI definition", []IndexPageLink{
		{Desc: "Swagger JSON", Path: "/api/swagger.json"},
	})
	// register grpc-gateway api
	a.RegisterRoutesWithPrefix("/api", a.grpcGatewayMux, false, true, "GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS")
	// register fgprof
	a.RegisterRoute("/debug/fgprof", fgprof.Handler(), false, true, "GET")
	// register static assets
	a.RegisterRoutesWithPrefix("/static/", http.FileServer(http.FS(staticFiles)), false, true, "GET")
	// register ui
	uiAssets, err := public.Assets()
	if err != nil {
		return fmt.Errorf("unable to initialize the ui: %w", err)
	}

	// The UI used to be at /ui, but now it's at /.
	a.RegisterRoutesWithPrefix("/ui", http.RedirectHandler("/", http.StatusFound), false, true, "GET")
	// All assets are served as static files
	a.RegisterRoutesWithPrefix("/assets/", http.FileServer(uiAssets), false, true, "GET")

	// register status service providing config and buildinfo at grpc gateway
	if err := statusv1.RegisterStatusServiceHandlerServer(context.Background(), a.grpcGatewayMux, statusService); err != nil {
		return err
	}
	a.indexPage.AddLinks(buildInfoWeight, "Build information", []IndexPageLink{
		{Desc: "Build information", Path: "/api/v1/status/buildinfo"},
	})
	a.indexPage.AddLinks(configWeight, "Current config", []IndexPageLink{
		{Desc: "Including the default values", Path: "/api/v1/status/config"},
		{Desc: "Only values that differ from the defaults", Path: "/api/v1/status/config/diff"},
		{Desc: "Default values", Path: "/api/v1/status/config/default"},
	})
	return nil
}

func (a *API) RegisterCatchAll() error {
	uiIndexHandler, err := public.NewIndexHandler(a.cfg.BaseURL)
	if err != nil {
		return fmt.Errorf("unable to initialize the ui: %w", err)
	}

	// Serve index to known paths
	// This should be kept in sync with routes in public/app/pages/routes.ts
	for _, path := range []string{"/", "/explore", "/comparison", "/comparison-diff"} {
		a.RegisterRoute(path, uiIndexHandler, false, true, "GET")
	}

	a.indexPage.AddLinks(defaultWeight, "User interface", []IndexPageLink{
		{Desc: "User interface", Path: "/"},
	})

	return nil
}

// RegisterRuntimeConfig registers the endpoints associates with the runtime configuration
func (a *API) RegisterRuntimeConfig(runtimeConfigHandler http.HandlerFunc, userLimitsHandler http.HandlerFunc) {
	a.RegisterRoute("/runtime_config", runtimeConfigHandler, false, true, "GET")
	a.RegisterRoute("/api/v1/tenant_limits", userLimitsHandler, true, true, "GET")
	a.indexPage.AddLinks(runtimeConfigWeight, "Current runtime config", []IndexPageLink{
		{Desc: "Entire runtime config (including overrides)", Path: "/runtime_config"},
		{Desc: "Only values that differ from the defaults", Path: "/runtime_config?mode=diff"},
	})
}

func (a *API) RegisterTenantSettings(ts *settings.TenantSettings) {
	settingsv1connect.RegisterSettingsServiceHandler(a.server.HTTP, ts, a.connectOptionsAuthRecovery()...)

	_, isUnimplemented := ts.CollectionRulesServiceHandler.(*settingsv1connect.UnimplementedCollectionRulesServiceHandler)
	if !isUnimplemented {
		settingsv1connect.RegisterCollectionRulesServiceHandler(a.server.HTTP, ts, a.connectOptionsAuthRecovery()...)
	}

	_, isUnimplemented = ts.RecordingRulesServiceHandler.(*settingsv1connect.UnimplementedRecordingRulesServiceHandler)
	if !isUnimplemented {
		settingsv1connect.RegisterRecordingRulesServiceHandler(a.server.HTTP, ts, a.connectOptionsAuthRecovery()...)
	}
}

// RegisterOverridesExporter registers the endpoints associated with the overrides exporter.
func (a *API) RegisterOverridesExporter(oe *exporter.OverridesExporter) {
	a.RegisterRoute("/overrides-exporter/ring", http.HandlerFunc(oe.RingHandler), false, true, "GET", "POST")
	a.indexPage.AddLinks(defaultWeight, "Overrides-exporter", []IndexPageLink{
		{Desc: "Ring status", Path: "/overrides-exporter/ring"},
	})
}

// RegisterDistributor registers the endpoints associated with the distributor.
func (a *API) RegisterDistributor(d *distributor.Distributor, multitenancyEnabled bool) {
	pyroscopeHandler := pyroscope.NewPyroscopeIngestHandler(d, a.logger)
	otlpHandler := otlp.NewOTLPIngestHandler(d, a.logger, multitenancyEnabled)

	a.RegisterRoute("/ingest", pyroscopeHandler, true, true, "POST")
	a.RegisterRoute("/pyroscope/ingest", pyroscopeHandler, true, true, "POST")
	pushv1connect.RegisterPusherServiceHandler(a.server.HTTP, d, a.connectOptionsAuthRecovery()...)
	a.RegisterRoute("/distributor/ring", d, false, true, "GET", "POST")
	a.indexPage.AddLinks(defaultWeight, "Distributor", []IndexPageLink{
		{Desc: "Ring status", Path: "/distributor/ring"},
	})

	a.RegisterRoute(
		"/opentelemetry.proto.collector.profiles.v1development.ProfilesService/Export",
		otlpHandler,
		true,
		true,
		"POST",
	)

	// TODO(@petethepig): implement http/protobuf and http/json support
	// a.RegisterRoute("/v1/profiles", otlpHandler, true, true, "POST")
}

// RegisterMemberlistKV registers the endpoints associated with the memberlist KV store.
func (a *API) RegisterMemberlistKV(pathPrefix string, kvs *memberlist.KVInitService) {
	a.RegisterRoute("/memberlist", MemberlistStatusHandler(pathPrefix, kvs), false, true, "GET")
	a.indexPage.AddLinks(memberlistWeight, "Memberlist", []IndexPageLink{
		{Desc: "Status", Path: "/memberlist"},
	})
}

// RegisterIngesterRing registers the ring UI page associated with the distributor for writes.
func (a *API) RegisterIngesterRing(r http.Handler) {
	a.RegisterRoute("/ring", r, false, true, "GET", "POST")
	a.indexPage.AddLinks(defaultWeight, "Ingester", []IndexPageLink{
		{Desc: "Ring status", Path: "/ring"},
	})
}

func (a *API) RegisterQuerierServiceHandler(svc querierv1connect.QuerierServiceHandler) {
	querierv1connect.RegisterQuerierServiceHandler(a.server.HTTP, svc, a.connectOptionsAuthLogRecovery()...)
}

func (a *API) RegisterVCSServiceHandler(svc vcsv1connect.VCSServiceHandler) {
	vcsv1connect.RegisterVCSServiceHandler(a.server.HTTP, svc, a.connectOptionsAuthLogRecovery()...)
}

func (a *API) RegisterPyroscopeHandlers(client querierv1connect.QuerierServiceClient) {
	handlers := querier.NewHTTPHandlers(client)
	a.RegisterRoute("/pyroscope/render", http.HandlerFunc(handlers.Render), true, true, "GET")
	a.RegisterRoute("/pyroscope/render-diff", http.HandlerFunc(handlers.RenderDiff), true, true, "GET")
	a.RegisterRoute("/pyroscope/label-values", http.HandlerFunc(handlers.LabelValues), true, true, "GET")
}

// RegisterIngester registers the endpoints associated with the ingester.
func (a *API) RegisterIngester(svc *ingester.Ingester) {
	ingesterv1connect.RegisterIngesterServiceHandler(a.server.HTTP, svc, a.connectOptionsAuthRecovery()...)
}

func (a *API) RegisterStoreGateway(svc *storegateway.StoreGateway) {
	storegatewayv1connect.RegisterStoreGatewayServiceHandler(a.server.HTTP, svc, a.connectOptionsAuthRecovery()...)

	a.indexPage.AddLinks(defaultWeight, "Store-gateway", []IndexPageLink{
		{Desc: "Ring status", Path: "/store-gateway/ring"},
		{Desc: "Tenants & Blocks", Path: "/store-gateway/tenants"},
	})
	a.RegisterRoute("/store-gateway/ring", http.HandlerFunc(svc.RingHandler), false, true, "GET", "POST")
	a.RegisterRoute("/store-gateway/tenants", http.HandlerFunc(svc.TenantsHandler), false, true, "GET")
	a.RegisterRoute("/store-gateway/tenant/{tenant}/blocks", http.HandlerFunc(svc.BlocksHandler), false, true, "GET")
}

// RegisterCompactor registers routes associated with the compactor.
func (a *API) RegisterCompactor(c *compactor.MultitenantCompactor) {
	a.indexPage.AddLinks(defaultWeight, "Compactor", []IndexPageLink{
		{Desc: "Ring status", Path: "/compactor/ring"},
	})
	a.RegisterRoute("/compactor/ring", http.HandlerFunc(c.RingHandler), false, true, "GET", "POST")
}

// RegisterFrontendForQuerierHandler registers the endpoints associated with the query frontend.
func (a *API) RegisterFrontendForQuerierHandler(frontendSvc *frontend.Frontend) {
	frontendpbconnect.RegisterFrontendForQuerierHandler(a.server.HTTP, frontendSvc, a.connectOptionsAuthRecovery()...)
}

// RegisterVersion registers the endpoints associated with the versions service.
func (a *API) RegisterVersion(svc versionv1connect.VersionHandler) {
	versionv1connect.RegisterVersionHandler(a.server.HTTP, svc, a.connectOptionsRecovery()...)
}

// RegisterQueryScheduler registers the endpoints associated with the query scheduler.
func (a *API) RegisterQueryScheduler(s *scheduler.Scheduler) {
	schedulerpbconnect.RegisterSchedulerForFrontendHandler(a.server.HTTP, s, a.connectOptionsRecovery()...)
	schedulerpbconnect.RegisterSchedulerForQuerierHandler(a.server.HTTP, s, a.connectOptionsRecovery()...)
}

// RegisterFlags registers api-related flags.
func (cfg *Config) RegisterFlags(fs *flag.FlagSet) {
	fs.StringVar(
		&cfg.BaseURL,
		"api.base-url",
		"",
		"base URL for when the server is behind a reverse proxy with a different path",
	)
}

func (a *API) RegisterAdmin(ad *operations.Admin) {
	a.RegisterRoute("/ops/object-store/tenants", http.HandlerFunc(ad.TenantsHandler), false, true, "GET")
	a.RegisterRoute("/ops/object-store/tenants/{tenant}/blocks", http.HandlerFunc(ad.BlocksHandler), false, true, "GET")
	a.RegisterRoute("/ops/object-store/tenants/{tenant}/blocks/{block}", http.HandlerFunc(ad.BlockHandler), false, true, "GET")

	a.indexPage.AddLinks(defaultWeight, "Admin", []IndexPageLink{
		{Desc: "Object Storage Tenants & Blocks", Path: "/ops/object-store/tenants"},
	})
}

func (a *API) RegisterAdHocProfiles(ahp *adhocprofiles.AdHocProfiles) {
	adhocprofilesv1connect.RegisterAdHocProfileServiceHandler(a.server.HTTP, ahp, a.connectOptionsAuthRecovery()...)
}

func (a *API) connectOptionsRecovery() []connect.HandlerOption {
	return append(connectapi.DefaultHandlerOptions(), a.recoveryMiddleware)
}

func (a *API) connectOptionsAuthRecovery() []connect.HandlerOption {
	return append(connectapi.DefaultHandlerOptions(), []connect.HandlerOption{a.grpcAuthMiddleware, a.recoveryMiddleware}...)
}

func (a *API) connectOptionsAuthLogRecovery() []connect.HandlerOption {
	return append(
		connectapi.DefaultHandlerOptions(),
		[]connect.HandlerOption{a.grpcAuthMiddleware, a.grpcLogMiddleware, a.recoveryMiddleware}...)
}
