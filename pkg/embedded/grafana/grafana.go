package grafana

import (
	"bufio"
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"
)

func mustHexDecode(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

var exploreProfileReleases = releaseArtifacts{
	{
		URL:          "https://github.com/grafana/profiles-drilldown/releases/download/v1.5.0/grafana-pyroscope-app-v1.5.0.zip",
		Sha256Sum:    mustHexDecode("0b74db5c96b5edc8849af9ca830ff98ba6eaff96f30deda225e6cd68514cc574"),
		CompressType: CompressTypeZip,
	},
}

var grafanaReleases = releaseArtifacts{
	{
		URL:             "https://dl.grafana.com/oss/release/grafana-12.0.2.linux-amd64.tar.gz",
		Sha256Sum:       mustHexDecode("c1755b4da918edfd298d5c8d5f1ffce35982ad10e1640ec356570cfb8c34b3e8"),
		OS:              "linux",
		Arch:            "amd64",
		CompressType:    CompressTypeGzip,
		StripComponents: 1,
	},
	{
		URL:             "https://dl.grafana.com/oss/release/grafana-12.0.2.linux-arm64.tar.gz",
		Sha256Sum:       mustHexDecode("bc0b186458cc91e2f96a06ecff2b3b4033b1a6ffd2449817e2a430a0b4ae4f12"),
		OS:              "linux",
		Arch:            "arm64",
		CompressType:    CompressTypeGzip,
		StripComponents: 1,
	},
	{
		URL:             "https://dl.grafana.com/oss/release/grafana-12.0.2.darwin-amd64.tar.gz",
		Sha256Sum:       mustHexDecode("ca2e97682149e482813c343f6b6d93e2f67dea33eae9a808513c7c62457c66e9"),
		OS:              "darwin",
		Arch:            "amd64",
		CompressType:    CompressTypeGzip,
		StripComponents: 1,
	},
	{
		URL:             "https://dl.grafana.com/oss/release/grafana-12.0.2.darwin-arm64.tar.gz",
		Sha256Sum:       mustHexDecode("3f162ba5dc3bbd0133055ecd40f4f1f791b287bee8e0487449f001b6e1c11a4f"),
		OS:              "darwin",
		Arch:            "arm64",
		CompressType:    CompressTypeGzip,
		StripComponents: 1,
	},
}

type app struct {
	cfg    Config
	logger log.Logger

	grafanaRelease        *releaseArtifact
	exploreProfileRelease *releaseArtifact

	dataPath         string
	pluginsPath      string
	provisioningPath string

	g *errgroup.Group
}

type Config struct {
	DataPath     string `yaml:"data_path" json:"data_path"`
	ListenPort   int    `yaml:"listen_port" json:"listen_port"`
	PyroscopeURL string `yaml:"pyroscope_url" json:"pyroscope_url"`
}

// RegisterFlags registers distributor-related flags.
func (cfg *Config) RegisterFlags(fs *flag.FlagSet) {
	fs.StringVar(&cfg.DataPath, "embedded-grafana.data-path", "./data/__embedded_grafana/", "The directory where the Grafana data will be stored.")
	fs.IntVar(&cfg.ListenPort, "embedded-grafana.listen-port", 4041, "The port on which the Grafana will listen.")
	fs.StringVar(&cfg.PyroscopeURL, "embedded-grafana.pyroscope-url", "http://localhost:4040", "The URL of the Pyroscope instance to use for the Grafana datasources.")
}

func New(cfg Config, logger log.Logger) (services.Service, error) {
	var err error
	cfg.DataPath, err = filepath.Abs(cfg.DataPath)
	if err != nil {
		return nil, err
	}

	grafanaRelease := grafanaReleases.selectBy(runtime.GOOS, runtime.GOARCH)
	if grafanaRelease == nil {
		return nil, fmt.Errorf("no Grafana release found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	exploreProfileRelease := exploreProfileReleases.selectBy(runtime.GOOS, runtime.GOARCH)
	if exploreProfileRelease == nil {
		level.Warn(logger).Log("msg", fmt.Sprintf("no Explore Profile plugin release found for %s/%s", runtime.GOOS, runtime.GOARCH))
	}

	a := &app{
		cfg:                   cfg,
		logger:                logger,
		grafanaRelease:        grafanaRelease,
		exploreProfileRelease: exploreProfileRelease,

		dataPath:         filepath.Join(cfg.DataPath, "data"),
		pluginsPath:      filepath.Join(cfg.DataPath, "plugins"),
		provisioningPath: filepath.Join(cfg.DataPath, "provisioning"),
	}
	return services.NewBasicService(a.starting, a.running, a.stopping), nil
}

func (a *app) downloadExploreProfiles(ctx context.Context) error {
	// download the explore-profiles plugin
	pluginPath, err := a.exploreProfileRelease.download(ctx, a.logger, a.cfg.DataPath)
	if err != nil {
		return err
	}

	// symlink the explore-profiles plugin to the plugins directory
	err = os.MkdirAll(a.pluginsPath, modeDir)
	if err != nil {
		return err
	}

	linkDest := filepath.Join(a.pluginsPath, "grafana-pyroscope-app")
	linkSource, err := filepath.Rel(a.pluginsPath, filepath.Join(pluginPath, "grafana-pyroscope-app"))
	if err != nil {
		return err
	}

	stat, err := os.Lstat(linkDest)
	if err == nil {
		if stat.Mode()&os.ModeSymlink == os.ModeSymlink {
			// already existing and symlink
			target, err := os.Readlink(filepath.Join(a.pluginsPath, "grafana-pyroscope-app"))
			if err != nil {
				return err
			}

			if target == linkSource {
				return nil
			}

			// recreate the symlink if it points to a different path
			err = os.Remove(linkDest)
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("file exists and is not a symlink: %+#v", stat)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	return os.Symlink(linkSource, linkDest)
}

func writeYAML(logger log.Logger, path string, data interface{}) error {
	err := os.MkdirAll(filepath.Dir(path), modeDir)
	if err != nil {
		return err
	}

	f, err := os.Create(path)
	defer func() {
		err := f.Close()
		if err != nil {
			level.Error(logger).Log("msg", "failed to close file", "path", path, "err", err)
		}
	}()
	if err != nil {
		return err
	}

	_, err = f.Write([]byte("# Note: Do not edit this file directly. It is managed by pyroscope.\n"))
	if err != nil {
		return err
	}

	yamlData, err := yaml.Marshal(data)
	if err != nil {
		return err
	}

	_, err = f.Write(yamlData)
	return err

}

func (a *app) provisioningDatasource(_ context.Context) error {
	return writeYAML(
		a.logger,
		filepath.Join(a.provisioningPath, "datasources", "embedded-grafana.yaml"),
		map[string]interface{}{
			"apiVersion": 1,
			"datasources": []interface{}{
				map[string]interface{}{
					"uid":  "pyroscope",
					"type": "grafana-pyroscope-datasource",
					"name": "Pyroscope",
					"url":  a.cfg.PyroscopeURL,
					"jsonData": map[string]interface{}{
						"keepCookies":      []string{"pyroscope_git_session"},
						"overridesDefault": true,
					},
				},
			},
		},
	)
}

func (a *app) provisioningPlugins(_ context.Context) error {
	return writeYAML(
		a.logger,
		filepath.Join(a.provisioningPath, "plugins", "embedded-grafana.yaml"),
		map[string]interface{}{
			"apiVersion": 1,
			"apps": []interface{}{
				map[string]interface{}{
					"type": "grafana-pyroscope-app",
				},
			},
		},
	)
}

func (a *app) starting(ctx context.Context) error {
	if a.exploreProfileRelease != nil {
		err := a.downloadExploreProfiles(ctx)
		if err != nil {
			return err
		}
	}

	err := a.provisioningDatasource(ctx)
	if err != nil {
		return err
	}

	err = a.provisioningPlugins(ctx)
	if err != nil {
		return err
	}

	grafanaPath, err := a.grafanaRelease.download(ctx, a.logger, a.cfg.DataPath)
	if err != nil {
		return err
	}

	cmd := exec.Command(
		filepath.Join(grafanaPath, "bin/grafana"),
		"server",
		"--homepath",
		grafanaPath,
	)
	cmd.Dir = a.cfg.DataPath
	cmd.Env = os.Environ()
	setIfNotExists := func(key, value string) {
		if os.Getenv(key) == "" {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
		}
	}
	setIfNotExists("GF_PLUGINS_PREINSTALL_DISABLED", "true") // required so we can load the version we have bundled
	setIfNotExists("GF_PATHS_DATA", a.dataPath)
	setIfNotExists("GF_PATHS_PLUGINS", a.pluginsPath)
	setIfNotExists("GF_PATHS_PROVISIONING", a.provisioningPath)
	setIfNotExists("GF_AUTH_ANONYMOUS_ENABLED", "true")
	setIfNotExists("GF_AUTH_ANONYMOUS_ORG_ROLE", "Admin")
	setIfNotExists("GF_AUTH_DISABLE_LOGIN_FORM", "true")
	setIfNotExists("GF_SERVER_HTTP_PORT", strconv.Itoa(a.cfg.ListenPort))
	setIfNotExists("GF_LOG_LEVEL", "error")

	a.g, _ = errgroup.WithContext(ctx)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	a.g.Go(func() error {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			level.Info(a.logger).Log("stream", "stdout", "msg", scanner.Text())
		}
		return scanner.Err()
	})

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	a.g.Go(func() error {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			level.Info(a.logger).Log("stream", "stderr", "msg", scanner.Text())
		}
		return scanner.Err()
	})

	if err = cmd.Start(); err != nil {
		return err
	}

	a.g.Go(func() error {
		<-ctx.Done()
		return cmd.Process.Signal(syscall.SIGINT)
	})

	a.g.Go(cmd.Wait)

	return nil
}

func (a *app) stopping(failureCase error) error {
	return nil
}

func (a *app) running(ctx context.Context) error {
	return a.g.Wait()
}
