package client

import (
	"errors"
	"flag"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-kit/log"
	"github.com/samber/lo"
	"github.com/thanos-io/objstore"

	"github.com/grafana/pyroscope/pkg/objstore/providers/azure"
	"github.com/grafana/pyroscope/pkg/objstore/providers/cos"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/pkg/objstore/providers/gcs"
	"github.com/grafana/pyroscope/pkg/objstore/providers/s3"
	"github.com/grafana/pyroscope/pkg/objstore/providers/swift"
)

const (
	// None is the null value for the storage backends.
	None = ""

	// S3 is the value for the S3 storage backend.
	S3 = "s3"

	// GCS is the value for the GCS storage backend.
	GCS = "gcs"

	// Azure is the value for the Azure storage backend.
	Azure = "azure"

	// Swift is the value for the Openstack Swift storage backend.
	Swift = "swift"

	// COS is the value for the Tencent Cloud COS storage backend.
	COS = "cos"

	// Filesystem is the value for the filesystem storage backend.
	Filesystem = "filesystem"

	// validPrefixCharactersRegex allows only alphanumeric characters to prevent subtle bugs and simplify validation
	validPrefixCharactersRegex = `^[\da-zA-Z]+$`
)

var (
	SupportedBackends = []string{S3, GCS, Azure, Swift, Filesystem, COS}

	ErrUnsupportedStorageBackend        = errors.New("unsupported storage backend")
	ErrInvalidCharactersInStoragePrefix = errors.New("storage prefix contains invalid characters, it may only contain digits and English alphabet letters")
)

type StorageBackendConfig struct {
	Backend string `yaml:"backend"`

	// Backends
	S3         s3.Config         `yaml:"s3"`
	GCS        gcs.Config        `yaml:"gcs"`
	Azure      azure.Config      `yaml:"azure"`
	Swift      swift.Config      `yaml:"swift"`
	COS        cos.Config        `yaml:"cos"`
	Filesystem filesystem.Config `yaml:"filesystem"`
}

// Returns the supportedBackends for the package and any custom backends injected into the config.
func (cfg *StorageBackendConfig) supportedBackends() []string {
	return SupportedBackends
}

// RegisterFlags registers the backend storage config.
func (cfg *StorageBackendConfig) RegisterFlags(f *flag.FlagSet, logger log.Logger) {
	cfg.RegisterFlagsWithPrefix("", f, logger)
}

func (cfg *StorageBackendConfig) RegisterFlagsWithPrefixAndDefaultDirectory(prefix, dir string, f *flag.FlagSet, logger log.Logger) {
	cfg.S3.RegisterFlagsWithPrefix(prefix, f)
	cfg.GCS.RegisterFlagsWithPrefix(prefix, f)
	cfg.Azure.RegisterFlagsWithPrefix(prefix, f)
	cfg.Swift.RegisterFlagsWithPrefix(prefix, f)
	cfg.Filesystem.RegisterFlagsWithPrefixAndDefaultDirectory(prefix, dir, f)
	cfg.COS.RegisterFlagsWithPrefix(prefix, f)
	f.StringVar(&cfg.Backend, prefix+"backend", None, fmt.Sprintf("Backend storage to use. Supported backends are: %s.", strings.Join(cfg.supportedBackends(), ", ")))
}

func (cfg *StorageBackendConfig) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet, logger log.Logger) {
	cfg.RegisterFlagsWithPrefixAndDefaultDirectory(prefix, "", f, logger)
}

func (cfg *StorageBackendConfig) Validate() error {
	if !lo.Contains(cfg.supportedBackends(), cfg.Backend) {
		return ErrUnsupportedStorageBackend
	}

	switch cfg.Backend {
	case S3:
		return cfg.S3.Validate()
	case COS:
		return cfg.COS.Validate()
	default:
		return nil
	}
}

// Config holds configuration for accessing long-term storage.
type Config struct {
	StorageBackendConfig `yaml:",inline"`

	StoragePrefix string `yaml:"storage_prefix" category:"experimental"`

	// Not used internally, meant to allow callers to wrap Buckets
	// created using this config
	Middlewares []func(objstore.Bucket) (objstore.Bucket, error) `yaml:"-"`
}

// RegisterFlags registers the backend storage config.
func (cfg *Config) RegisterFlags(f *flag.FlagSet, logger log.Logger) {
	cfg.RegisterFlagsWithPrefix("", f, logger)
}

func (cfg *Config) RegisterFlagsWithPrefixAndDefaultDirectory(prefix, dir string, f *flag.FlagSet, logger log.Logger) {
	cfg.StorageBackendConfig.RegisterFlagsWithPrefixAndDefaultDirectory(prefix, dir, f, logger)
	f.StringVar(&cfg.StoragePrefix, prefix+"storage-prefix", "", "Prefix for all objects stored in the backend storage. For simplicity, it may only contain digits and English alphabet letters.")
}

func (cfg *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet, logger log.Logger) {
	cfg.RegisterFlagsWithPrefixAndDefaultDirectory(prefix, "./data-shared", f, logger)
}

func (cfg *Config) Validate() error {
	if cfg.StoragePrefix != "" {
		acceptablePrefixCharacters := regexp.MustCompile(validPrefixCharactersRegex)
		if !acceptablePrefixCharacters.MatchString(cfg.StoragePrefix) {
			return ErrInvalidCharactersInStoragePrefix
		}
	}

	return cfg.StorageBackendConfig.Validate()
}
