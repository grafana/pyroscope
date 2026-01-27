package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-kit/log/level"
)

type kubeProxyParams struct {
	*phlareClient
	Context       string
	Namespace     string
	LabelSelector string
	ListenAddr    string
}

type pyroscopeService struct {
	Name      string // e.g., "pyroscope-distributor"
	Component string // e.g., "distributor"
	Port      int    // Service port
	PortName  string // e.g., "http"
}

type routingRule struct {
	PathPrefix string // e.g., "/ingest"
	Component  string // e.g., "distributor"
}

type reverseProxyManager struct {
	params            *kubeProxyParams
	services          map[string]*pyroscopeService // component -> service
	kubectlProcess    *exec.Cmd
	kubectlSocketPath string
	routingRules      []routingRule
	httpServer        *http.Server
	ctx               context.Context
	cancel            context.CancelFunc
}

func addKubeProxyParams(cmd commander) *kubeProxyParams {
	params := &kubeProxyParams{}
	params.phlareClient = addPhlareClient(cmd)

	cmd.Flag("context", "Kubernetes context to use").Short('c').Default("").Envar("KUBERNETES_CONTEXT").StringVar(&params.Context)
	cmd.Flag("namespace", "Kubernetes namespace").Short('n').Default("default").Envar("KUBERNETES_NAMESPACE").StringVar(&params.Namespace)
	cmd.Flag("label-selector", "Label selector for Pyroscope services").Short('l').
		Default("").StringVar(&params.LabelSelector)
	cmd.Flag("listen-addr", "Address to listen on (host:port)").
		Default("127.0.0.1:4242").StringVar(&params.ListenAddr)

	return params
}

func kubeProxyCommand(ctx context.Context, params *kubeProxyParams) error {
	mgr := &reverseProxyManager{
		params: params,
	}
	mgr.ctx, mgr.cancel = context.WithCancel(ctx)
	defer mgr.cancel()

	// Discover services
	if err := mgr.discoverServices(); err != nil {
		return fmt.Errorf("service discovery failed: %w", err)
	}

	// Start kubectl proxy in background
	if err := mgr.startKubectlProxy(); err != nil {
		return fmt.Errorf("failed to start kubectl proxy: %w", err)
	}

	// Initialize routing rules
	mgr.initRoutingRules()

	// Start reverse proxy server
	if err := mgr.startReverseProxy(); err != nil {
		mgr.shutdown()
		return fmt.Errorf("failed to start reverse proxy: %w", err)
	}

	// Print connection info
	mgr.printConnectionInfo()

	// Wait for shutdown signal
	return mgr.run()
}

func (m *reverseProxyManager) discoverServices() error {
	args := []string{
		"get", "services",
		"--selector", m.params.LabelSelector,
		"-o", "json",
	}

	if m.params.Context != "" {
		args = append([]string{"--context", m.params.Context}, args...)
	}
	args = append([]string{"--namespace", m.params.Namespace}, args...)

	cmd := exec.Command("kubectl", args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("kubectl failed: %s", string(exitErr.Stderr))
		}
		return fmt.Errorf("failed to run kubectl: %w", err)
	}

	var serviceList struct {
		Items []struct {
			Metadata struct {
				Name   string            `json:"name"`
				Labels map[string]string `json:"labels"`
			} `json:"metadata"`
			Spec struct {
				ClusterIP string `json:"clusterIP"`
				Ports     []struct {
					Name string `json:"name"`
					Port int    `json:"port"`
				} `json:"ports"`
			} `json:"spec"`
		} `json:"items"`
	}

	if err := json.Unmarshal(output, &serviceList); err != nil {
		return fmt.Errorf("failed to parse services JSON: %w", err)
	}

	// Filter to user-facing services only
	allowedComponents := map[string]bool{
		"distributor":     true,
		"query-frontend":  true,
		"tenant-settings": true,
		"ad-hoc-profiles": true,
	}

	m.services = make(map[string]*pyroscopeService)

	for _, item := range serviceList.Items {
		// Skip headless services
		if item.Spec.ClusterIP == "None" {
			level.Debug(logger).Log("msg", "skipping headless service", "name", item.Metadata.Name)
			continue
		}

		component := item.Metadata.Labels["app.kubernetes.io/component"]
		if component == "" || !allowedComponents[component] {
			continue
		}

		// Find HTTP port
		var port int
		var portName string
		for _, p := range item.Spec.Ports {
			if p.Name == "http" || p.Name == "http-metrics" {
				port = p.Port
				portName = p.Name
				break
			}
		}
		if port == 0 && len(item.Spec.Ports) > 0 {
			port = item.Spec.Ports[0].Port
			portName = item.Spec.Ports[0].Name
		}

		m.services[component] = &pyroscopeService{
			Name:      item.Metadata.Name,
			Component: component,
			Port:      port,
			PortName:  portName,
		}

		level.Info(logger).Log("msg", "discovered service", "name", item.Metadata.Name, "component", component)
	}

	if len(m.services) == 0 {
		return fmt.Errorf("no user-facing Pyroscope services found (looking for: distributor, query-frontend, tenant-settings, ad-hoc-profiles)")
	}

	return nil
}

func (m *reverseProxyManager) startKubectlProxy() error {
	// Create Unix socket path for kubectl proxy
	m.kubectlSocketPath = "/tmp/pyroscope-kubectl-proxy.sock"

	// Remove socket if it already exists
	if _, err := os.Stat(m.kubectlSocketPath); err == nil {
		if err := os.Remove(m.kubectlSocketPath); err != nil {
			return fmt.Errorf("failed to remove existing kubectl socket: %w", err)
		}
	}

	args := []string{
		"proxy",
		"--unix-socket", m.kubectlSocketPath,
		"--disable-filter=true",
	}

	if m.params.Context != "" {
		args = append([]string{"--context", m.params.Context}, args...)
	}

	cmd := exec.CommandContext(m.ctx, "kubectl", args...)

	// Capture stderr for error reporting
	stderr := &strings.Builder{}
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start kubectl proxy: %w", err)
	}

	m.kubectlProcess = cmd
	level.Info(logger).Log("msg", "started kubectl proxy", "socket", m.kubectlSocketPath, "pid", cmd.Process.Pid)

	// Wait for kubectl proxy to be ready
	if err := m.waitForKubectlProxy(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("%w (stderr: %s)", err, stderr.String())
		}
		return err
	}

	// Set appropriate permissions on the socket (user only)
	if err := os.Chmod(m.kubectlSocketPath, 0600); err != nil {
		level.Warn(logger).Log("msg", "failed to set kubectl socket permissions", "err", err)
	}

	return nil
}

func (m *reverseProxyManager) waitForKubectlProxy() error {
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", m.kubectlSocketPath)
			},
		},
		Timeout: 2 * time.Second,
	}

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for kubectl proxy to start")
		case <-ticker.C:
			// Check if socket exists
			if _, err := os.Stat(m.kubectlSocketPath); err != nil {
				continue
			}

			// Try to connect
			resp, err := client.Get("http://unix/api")
			if err == nil {
				resp.Body.Close()
				level.Info(logger).Log("msg", "kubectl proxy is ready")
				return nil
			}
		}
	}
}

func (m *reverseProxyManager) initRoutingRules() {
	// Order matters - most specific first!
	m.routingRules = []routingRule{
		// Distributor (write path)
		{PathPrefix: "/push.v1.PusherService/", Component: "distributor"},
		{PathPrefix: "/ingest", Component: "distributor"},
		{PathPrefix: "/pyroscope/ingest", Component: "distributor"},
		{PathPrefix: "/v1development/profiles", Component: "distributor"},
		{PathPrefix: "/opentelemetry.proto.collector.profiles", Component: "distributor"},

		// Tenant Settings
		{PathPrefix: "/settings.v1.SettingsService/", Component: "tenant-settings"},
		{PathPrefix: "/settings.v1.RecordingRulesService/", Component: "tenant-settings"},

		// Ad-Hoc Profiles
		{PathPrefix: "/adhocprofiles.v1.AdHocProfileService/", Component: "ad-hoc-profiles"},

		// Default to query-frontend for root and UI paths
		{PathPrefix: "/", Component: "query-frontend"},
	}
}

func (m *reverseProxyManager) startReverseProxy() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", m.handleRequest)

	m.httpServer = &http.Server{
		Handler: mux,
	}

	listener, err := net.Listen("tcp", m.params.ListenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", m.params.ListenAddr, err)
	}

	level.Info(logger).Log("msg", "reverse proxy listening", "addr", m.params.ListenAddr)

	// Start serving in background
	go func() {
		if err := m.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			level.Error(logger).Log("msg", "HTTP server error", "err", err)
		}
	}()

	return nil
}

func (m *reverseProxyManager) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Route to appropriate service
	svc, err := m.routeRequest(r)
	if err != nil {
		level.Warn(logger).Log("msg", "routing failed", "err", err, "path", r.URL.Path)
		http.Error(w, fmt.Sprintf("Routing error: %v", err), http.StatusBadGateway)
		return
	}

	level.Debug(logger).Log("msg", "routing request", "method", r.Method, "path", r.URL.Path, "service", svc.Name, "component", svc.Component)

	// Proxy to service via kubectl
	m.proxyToService(w, r, svc)
}

func (m *reverseProxyManager) routeRequest(r *http.Request) (*pyroscopeService, error) {
	path := r.URL.Path

	// Check each routing rule
	for _, rule := range m.routingRules {
		if strings.HasPrefix(path, rule.PathPrefix) {
			// Find service for this component
			svc, ok := m.services[rule.Component]
			if !ok {
				return nil, fmt.Errorf("service not found for component: %s", rule.Component)
			}

			return svc, nil
		}
	}

	return nil, fmt.Errorf("no routing rule matched for path: %s", path)
}

func (m *reverseProxyManager) proxyToService(w http.ResponseWriter, r *http.Request, svc *pyroscopeService) {
	// Build target URL via kubectl proxy (using unix socket)
	targetPath := fmt.Sprintf(
		"/api/v1/namespaces/%s/services/%s:%s/proxy%s",
		m.params.Namespace,
		svc.Name,
		svc.PortName,
		r.URL.Path,
	)

	// Add query string if present
	if r.URL.RawQuery != "" {
		targetPath += "?" + r.URL.RawQuery
	}

	targetURL := "http://unix" + targetPath

	// Create proxy request
	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		level.Error(logger).Log("msg", "failed to create proxy request", "err", err)
		http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
		return
	}

	// Copy headers, but skip Authorization to avoid interfering with kubectl's auth
	for key, values := range r.Header {
		// Skip Authorization header - kubectl proxy uses kubeconfig auth, not incoming auth
		if key == "Authorization" {
			level.Debug(logger).Log("msg", "skipping Authorization header from incoming request")
			continue
		}
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}

	// Inject tenant ID if configured
	if m.params.TenantID != "" {
		proxyReq.Header.Set("X-Scope-OrgID", m.params.TenantID)
		level.Debug(logger).Log("msg", "injected tenant ID", "tenant_id", m.params.TenantID)
	}

	// Execute request via Unix socket
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", m.kubectlSocketPath)
			},
		},
		Timeout: 5 * time.Minute,
	}

	resp, err := client.Do(proxyReq)
	if err != nil {
		level.Error(logger).Log("msg", "proxy request failed", "err", err, "path", targetPath)
		http.Error(w, fmt.Sprintf("Proxy error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Copy status code
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	io.Copy(w, resp.Body)
}

func (m *reverseProxyManager) printConnectionInfo() {
	baseURL := fmt.Sprintf("curl http://%s", m.params.ListenAddr)

	fmt.Fprintf(output(m.ctx), "\nPyroscope Kubernetes Reverse Proxy\n")
	fmt.Fprintf(output(m.ctx), "===================================\n\n")
	fmt.Fprintf(output(m.ctx), "Context:    %s\n", m.params.Context)
	fmt.Fprintf(output(m.ctx), "Namespace:  %s\n", m.params.Namespace)
	fmt.Fprintf(output(m.ctx), "Services:   %d discovered\n", len(m.services))
	fmt.Fprintf(output(m.ctx), "Listening:  %s\n\n", m.params.ListenAddr)

	fmt.Fprintf(output(m.ctx), "Routing Examples:\n")
	fmt.Fprintf(output(m.ctx), "-----------------\n\n")

	// Show routing examples for each service
	type example struct {
		title string
		cmds  []string
	}

	examples := map[string]example{
		"query-frontend": {
			title: "Query profiles:",
			cmds: []string{
				fmt.Sprintf("  %s/render?query=process_cpu&from=now-1h", baseURL),
				fmt.Sprintf("  %s/querier.v1.QuerierService/ProfileTypes -X POST -d '{}' -H 'content-type: application/json'", baseURL),
			},
		},
		"distributor": {
			title: "Push profiles:",
			cmds: []string{
				fmt.Sprintf("  %s/ingest -X POST -d @profile.pprof", baseURL),
			},
		},
		"tenant-settings": {
			title: "Manage settings:",
			cmds: []string{
				fmt.Sprintf("  %s/settings.v1.SettingsService/Get -X POST -d '{}' -H 'content-type: application/json'", baseURL),
			},
		},
		"ad-hoc-profiles": {
			title: "Ad-hoc profiles:",
			cmds: []string{
				fmt.Sprintf("  %s/adhocprofiles.v1.AdHocProfileService/List -X POST -d '{}' -H 'content-type: application/json'", baseURL),
			},
		},
	}

	// Display in logical order
	for _, component := range []string{"query-frontend", "distributor", "tenant-settings", "ad-hoc-profiles"} {
		if _, ok := m.services[component]; !ok {
			continue
		}

		if ex, ok := examples[component]; ok {
			fmt.Fprintf(output(m.ctx), "%s\n", ex.title)
			for _, cmd := range ex.cmds {
				fmt.Fprintf(output(m.ctx), "%s\n", cmd)
			}
			fmt.Fprintf(output(m.ctx), "\n")
		}
	}

	fmt.Fprintf(output(m.ctx), "The proxy automatically routes requests to the correct service!\n")
	fmt.Fprintf(output(m.ctx), "\nPress Ctrl+C to stop\n")
}

func (m *reverseProxyManager) run() error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	processDone := make(chan error, 1)
	go func() {
		processDone <- m.kubectlProcess.Wait()
	}()

	select {
	case sig := <-sigChan:
		level.Info(logger).Log("msg", "received signal", "signal", sig)
		return m.shutdown()
	case err := <-processDone:
		if err != nil {
			level.Error(logger).Log("msg", "kubectl proxy exited unexpectedly", "err", err)
		}
		m.shutdown()
		return err
	}
}

func (m *reverseProxyManager) shutdown() error {
	level.Info(logger).Log("msg", "shutting down")

	// Stop HTTP server
	if m.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := m.httpServer.Shutdown(ctx); err != nil {
			level.Warn(logger).Log("msg", "HTTP server shutdown error", "err", err)
		}
	}

	// Stop kubectl proxy
	if m.kubectlProcess != nil && m.kubectlProcess.Process != nil {
		if err := m.kubectlProcess.Process.Signal(syscall.SIGTERM); err != nil {
			level.Warn(logger).Log("msg", "failed to send SIGTERM to kubectl", "err", err)
			m.kubectlProcess.Process.Kill()
		}

		done := make(chan error, 1)
		go func() {
			done <- m.kubectlProcess.Wait()
		}()

		select {
		case <-time.After(5 * time.Second):
			level.Warn(logger).Log("msg", "kubectl proxy didn't stop, killing")
			m.kubectlProcess.Process.Kill()
		case <-done:
		}
	}

	// Clean up kubectl socket
	if m.kubectlSocketPath != "" {
		if err := os.Remove(m.kubectlSocketPath); err != nil && !os.IsNotExist(err) {
			level.Warn(logger).Log("msg", "failed to remove kubectl socket", "err", err)
		} else {
			level.Info(logger).Log("msg", "removed kubectl socket", "path", m.kubectlSocketPath)
		}
	}

	level.Info(logger).Log("msg", "shutdown complete")
	return nil
}
