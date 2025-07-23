package client

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
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
)

var (
	SupportedBackends = []string{S3, GCS, Azure, Swift, Filesystem, COS}

	ErrUnsupportedStorageBackend      = errors.New("unsupported storage backend")
	ErrStoragePrefixStartsWithSlash   = errors.New("storage prefix starts with a slash")
	ErrStoragePrefixEmptyPathSegment  = errors.New("storage prefix contains an empty path segment")
	ErrStoragePrefixInvalidCharacters = errors.New("storage prefix contains invalid characters: only alphanumeric, hyphen, underscore, dot, and no segement should be '.' or '..'")
	ErrStoragePrefixBothFlagsSet      = errors.New("both storage.prefix and storage.storage-prefix are set, please use only storage.prefix, as storage.storage-prefix is deprecated")
)

type ErrInvalidCharactersInStoragePrefix struct {
	Prefix string
}

func (e *ErrInvalidCharactersInStoragePrefix) Error() string {
	return fmt.Sprintf("storage prefix '%s' contains invalid characters", e.Prefix)
}

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
func (cfg *StorageBackendConfig) RegisterFlags(f *flag.FlagSet) {
	cfg.RegisterFlagsWithPrefix("", f)
}

func (cfg *StorageBackendConfig) RegisterFlagsWithPrefixAndDefaultDirectory(prefix, dir string, f *flag.FlagSet) {
	cfg.S3.RegisterFlagsWithPrefix(prefix, f)
	cfg.GCS.RegisterFlagsWithPrefix(prefix, f)
	cfg.Azure.RegisterFlagsWithPrefix(prefix, f)
	cfg.Swift.RegisterFlagsWithPrefix(prefix, f)
	cfg.Filesystem.RegisterFlagsWithPrefixAndDefaultDirectory(prefix, dir, f)
	cfg.COS.RegisterFlagsWithPrefix(prefix, f)
	f.StringVar(&cfg.Backend, prefix+"backend", None, fmt.Sprintf("Backend storage to use. Supported backends are: %s.", strings.Join(cfg.supportedBackends(), ", ")))
}

func (cfg *StorageBackendConfig) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	cfg.RegisterFlagsWithPrefixAndDefaultDirectory(prefix, "", f)
}

func (cfg *StorageBackendConfig) Validate() error {
	if cfg.Backend == None {
		return nil
	}

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

	Prefix                  string `yaml:"prefix"`
	DeprecatedStoragePrefix string `yaml:"storage_prefix" category:"experimental"` // Deprecated: use Prefix instead

	// Not used internally, meant to allow callers to wrap Buckets
	// created using this config
	Middlewares []func(objstore.Bucket) (objstore.Bucket, error) `yaml:"-"`
}

// RegisterFlags registers the backend storage config.
func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	cfg.RegisterFlagsWithPrefix("", f)
}

func (cfg *Config) RegisterFlagsWithPrefixAndDefaultDirectory(prefix, dir string, f *flag.FlagSet) {
	cfg.StorageBackendConfig.RegisterFlagsWithPrefixAndDefaultDirectory(prefix, dir, f)
	f.StringVar(&cfg.Prefix, prefix+"prefix", "", "Prefix for all objects stored in the backend storage. For simplicity, it may only contain digits and English alphabet characters, hyphens, underscores, dots and forward slashes.")
	f.StringVar(&cfg.DeprecatedStoragePrefix, prefix+"storage-prefix", "", "Deprecated: Use '"+prefix+".prefix' instead. Prefix for all objects stored in the backend storage. For simplicity, it may only contain digits and English alphabet characters, hyphens, underscores, dots and forward slashes.")
}

func (cfg *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	cfg.RegisterFlagsWithPrefixAndDefaultDirectory(prefix, "./data-shared", f)
}

// alphanumeric, hyphen, underscore, dot, and must not be . or ..
func validStoragePrefixPart(part string) bool {
	if part == "." || part == ".." {
		return false
	}
	for i, b := range part {
		if (b < 'a' || b > 'z') && (b < 'A' || b > 'Z') && b != '_' && b != '-' && b != '.' && (b < '0' || b > '9' || i == 0) {
			return false
		}
	}
	return true
}

func validStoragePrefix(prefix string) error {
	// without a prefix exit quickly
	if prefix == "" {
		return nil
	}

	parts := strings.Split(prefix, "/")

	for idx, part := range parts {
		if part == "" {
			if idx == 0 {
				return ErrStoragePrefixStartsWithSlash
			}
			if idx != len(parts)-1 {
				return ErrStoragePrefixEmptyPathSegment
			}
			// slash at the end is fine
		}
		if !validStoragePrefixPart(part) {
			return ErrStoragePrefixInvalidCharacters
		}
	}

	return nil
}

func (cfg *Config) getPrefix() string {
	if cfg.Prefix != "" {
		return cfg.Prefix
	}

	return cfg.DeprecatedStoragePrefix
}

func (cfg *Config) Validate(logger log.Logger) error {
	if cfg.DeprecatedStoragePrefix != "" {
		if cfg.Prefix != "" {
			return ErrStoragePrefixBothFlagsSet
		}
		level.Warn(logger).Log("msg", "bucket config has a deprecated storage.storage-prefix flag set. Please, use storage.prefix instead.")
	}
	if err := validStoragePrefix(cfg.getPrefix()); err != nil {
		return err
	}

	return cfg.StorageBackendConfig.Validate()
}
