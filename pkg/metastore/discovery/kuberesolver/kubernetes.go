// nolint
package kuberesolver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	serviceAccountToken     = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	serviceAccountCACert    = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	kubernetesNamespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
	defaultNamespace        = "default"
)

// K8sClient is minimal kubernetes client interface
type K8sClient interface {
	Do(req *http.Request) (*http.Response, error)
	GetRequest(url string) (*http.Request, error)
	Host() string
}

// tokenRefreshPeriod is how long a cached service account token is
// used before it is re-read from disk. Projected service account tokens
// are rotated by the kubelet, and fsnotify events are not reliable
// enough to be the only refresh mechanism: if an event is missed or the
// watcher fails, the in-memory token is never replaced and every API
// request starts failing with 401 until the pod restarts.
const tokenRefreshPeriod = time.Minute

type k8sClient struct {
	host        string
	token       string
	tokenFile   string
	tokenReadAt time.Time
	tokenLck    sync.RWMutex
	httpClient  *http.Client
}

func (kc *k8sClient) GetRequest(url string) (*http.Request, error) {
	if !strings.HasPrefix(url, kc.host) {
		url = fmt.Sprintf("%s/%s", kc.host, url)
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if token := kc.currentToken(); len(token) > 0 {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}

// currentToken returns the cached service account token, re-reading it
// from disk when the cached copy is older than tokenRefreshPeriod. A
// failed re-read is not an error: the cached token is kept and the next
// request retries.
func (kc *k8sClient) currentToken() string {
	kc.tokenLck.RLock()
	token, readAt, tokenFile := kc.token, kc.tokenReadAt, kc.tokenFile
	kc.tokenLck.RUnlock()
	if tokenFile == "" || time.Since(readAt) < tokenRefreshPeriod {
		return token
	}
	// Serialize the re-read so that concurrent requests refresh at most
	// once per period, and a delayed disk read cannot clobber a newer
	// token delivered via fsnotify.
	kc.tokenLck.Lock()
	defer kc.tokenLck.Unlock()
	if time.Since(kc.tokenReadAt) < tokenRefreshPeriod {
		return kc.token
	}
	refreshed, err := os.ReadFile(tokenFile)
	if err != nil {
		// Keep the cached token and retry after the next period
		// instead of hitting the filesystem on every request.
		kc.tokenReadAt = time.Now()
		return kc.token
	}
	kc.token = string(refreshed)
	kc.tokenReadAt = time.Now()
	return kc.token
}

func (kc *k8sClient) Do(req *http.Request) (*http.Response, error) {
	return kc.httpClient.Do(req)
}

func (kc *k8sClient) Host() string {
	return kc.host
}

func (kc *k8sClient) setToken(token string) {
	kc.tokenLck.Lock()
	defer kc.tokenLck.Unlock()
	kc.token = token
	kc.tokenReadAt = time.Now()
}

// NewInClusterK8sClient creates K8sClient if it is inside Kubernetes
func NewInClusterK8sClient() (K8sClient, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if len(host) == 0 || len(port) == 0 {
		return nil, fmt.Errorf("unable to load in-cluster configuration, KUBERNETES_SERVICE_HOST and KUBERNETES_SERVICE_PORT must be defined")
	}
	token, err := os.ReadFile(serviceAccountToken)
	if err != nil {
		return nil, err
	}
	ca, err := os.ReadFile(serviceAccountCACert)
	if err != nil {
		return nil, err
	}
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(ca)
	transport := &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    certPool,
	}}
	httpClient := &http.Client{Transport: transport, Timeout: time.Nanosecond * 0}

	client := &k8sClient{
		host:        "https://" + net.JoinHostPort(host, port),
		token:       string(token),
		tokenFile:   serviceAccountToken,
		tokenReadAt: time.Now(),
		httpClient:  httpClient,
	}

	// Create a new file watcher to listen for new Service Account tokens
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// k8s configmaps uses symlinks, we need this workaround.
				// original configmap file is removed
				if event.Op.Has(fsnotify.Remove) || event.Op.Has(fsnotify.Chmod) {
					// remove watcher since the file is removed
					watcher.Remove(event.Name)
					// add a new watcher pointing to the new symlink/file
					watcher.Add(serviceAccountToken)
					token, err := os.ReadFile(serviceAccountToken)
					if err == nil {
						client.setToken(string(token))
					}
				}
				if event.Has(fsnotify.Write) {
					token, err := os.ReadFile(serviceAccountToken)
					if err == nil {
						client.setToken(string(token))
					}
				}
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()

	err = watcher.Add(serviceAccountToken)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// NewInsecureK8sClient creates an insecure k8s client which is suitable
// to connect kubernetes api behind proxy
func NewInsecureK8sClient(apiURL string) K8sClient {
	return &k8sClient{
		host:       apiURL,
		httpClient: http.DefaultClient,
	}
}

func getEndpoints(client K8sClient, namespace, targetName string) (Endpoints, error) {
	u, err := url.Parse(fmt.Sprintf("%s/api/v1/namespaces/%s/endpoints/%s",
		client.Host(), namespace, targetName))
	if err != nil {
		return Endpoints{}, err
	}
	req, err := client.GetRequest(u.String())
	if err != nil {
		return Endpoints{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Endpoints{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Endpoints{}, fmt.Errorf("invalid response code %d for service %s in namespace %s", resp.StatusCode, targetName, namespace)
	}
	result := Endpoints{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	return result, err
}

func watchEndpoints(ctx context.Context, client K8sClient, namespace, targetName string) (watchInterface, error) {
	u, err := url.Parse(fmt.Sprintf("%s/api/v1/watch/namespaces/%s/endpoints/%s",
		client.Host(), namespace, targetName))
	if err != nil {
		return nil, err
	}
	req, err := client.GetRequest(u.String())
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, fmt.Errorf("invalid response code %d for service %s in namespace %s", resp.StatusCode, targetName, namespace)
	}
	return newStreamWatcher(resp.Body), nil
}

func getCurrentNamespaceOrDefault() string {
	ns, err := os.ReadFile(kubernetesNamespaceFile)
	if err != nil {
		return defaultNamespace
	}
	return string(ns)
}
