package grafana

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

type CompressType int

const (
	CompressTypeNone CompressType = iota
	CompressTypeGzip
	CompressTypeZip
)

const (
	modeDir  = 0755
	modeFile = 0644
)

type releaseArtifacts []releaseArtifact

func (releases releaseArtifacts) selectBy(os, arch string) *releaseArtifact {
	var nonArch *releaseArtifact
	for idx, r := range releases {
		if r.OS == "" && r.Arch == "" && nonArch == nil {
			nonArch = &releases[idx]
			continue
		}
		if r.OS == os && r.Arch == arch {
			return &r
		}
	}
	return nonArch
}

type releaseArtifact struct {
	URL             string
	Sha256Sum       []byte
	OS              string
	Arch            string
	CompressType    CompressType
	StripComponents int
}

func (release *releaseArtifact) download(ctx context.Context, logger log.Logger, destPath string) (string, error) {
	targetPath := filepath.Join(destPath, "assets", hex.EncodeToString(release.Sha256Sum))

	// check if already exists
	if len(release.Sha256Sum) > 0 {
		stat, err := os.Stat(targetPath)
		if err != nil {
			if !os.IsNotExist(err) {
				return "", err
			}
		}
		if err == nil && stat.IsDir() {
			level.Info(logger).Log("msg", "release exists already", "url", release.URL, "hash", hex.EncodeToString(release.Sha256Sum))
			return targetPath, nil
		}
	}

	level.Info(logger).Log("msg", "download new release", "url", release.URL)
	req, err := http.NewRequestWithContext(ctx, "GET", release.URL, nil)
	req.Header.Set("User-Agent", "pyroscope/embedded-grafana")
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	file, err := os.CreateTemp("", "pyroscope-download")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())

	hash := sha256.New()
	r := io.TeeReader(resp.Body, hash)

	_, err = io.Copy(file, r)
	if err != nil {
		return "", err
	}

	err = file.Close()
	if err != nil {
		return "", err
	}

	actHashSum := hex.EncodeToString(hash.Sum(nil))
	if expHashSum := hex.EncodeToString(release.Sha256Sum); actHashSum != expHashSum {
		return "", fmt.Errorf("hash mismatch: expected %s, got %s", expHashSum, actHashSum)
	}

	switch release.CompressType {
	case CompressTypeNone:
		return targetPath, os.Rename(file.Name(), targetPath)
	case CompressTypeGzip:
		file, err = os.Open(file.Name())
		if err != nil {
			return "", err
		}
		defer file.Close()

		err = extractTarGz(file, targetPath, release.StripComponents)
		if err != nil {
			return "", err
		}
	case CompressTypeZip:
		file, err = os.Open(file.Name())
		if err != nil {
			return "", err
		}
		defer file.Close()

		stat, err := file.Stat()
		if err != nil {
			return "", err
		}

		err = extractZip(file, stat.Size(), targetPath, release.StripComponents)
		if err != nil {
			return "", err
		}
	}

	return targetPath, nil
}

func clearPath(name string, destPath string, stripComponents int) string {
	isSeparator := func(r rune) bool {
		return r == os.PathSeparator
	}
	list := strings.FieldsFunc(name, isSeparator)
	if len(list) > stripComponents {
		list = list[stripComponents:]
	}
	return filepath.Join(append([]string{destPath}, list...)...)
}

func extractZip(zipStream io.ReaderAt, size int64, destPath string, stripComponents int) error {
	zipReader, err := zip.NewReader(zipStream, size)
	if err != nil {
		return fmt.Errorf("ExtractZip: NewReader failed: %s", err.Error())
	}

	for _, f := range zipReader.File {
		p := clearPath(f.Name, destPath, stripComponents)
		if f.FileInfo().IsDir() {
			err := os.MkdirAll(p, modeDir)
			if err != nil {
				return fmt.Errorf("ExtractZip: MkdirAll() failed: %s", err.Error())
			}
			continue
		}

		dir, _ := filepath.Split(p)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, modeDir); err != nil {
				return fmt.Errorf("ExtractZip: MkdirAll() failed: %s", err.Error())
			}
		}

		fileInArchive, err := f.Open()
		if err != nil {
			return fmt.Errorf("ExtractZip: Open() failed: %s", err.Error())
		}

		outFile, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE|os.O_TRUNC, f.FileInfo().Mode())
		if err != nil {
			return fmt.Errorf("ExtractZip: OpenFile() failed: %s", err.Error())
		}
		if _, err := io.Copy(outFile, fileInArchive); err != nil {
			return fmt.Errorf("ExtractZip: Copy() failed: %s", err.Error())
		}
	}

	return nil

}

func extractTarGz(gzipStream io.Reader, destPath string, stripComponents int) error {
	uncompressedStream, err := gzip.NewReader(gzipStream)
	if err != nil {
		return errors.New("ExtractTarGz: NewReader failed")
	}

	tarReader := tar.NewReader(uncompressedStream)

	for {
		header, err := tarReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return fmt.Errorf("ExtractTarGz: Next() failed: %s", err.Error())
		}

		p := clearPath(header.Name, destPath, stripComponents)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(p, modeDir); err != nil {
				return fmt.Errorf("ExtractTarGz: Mkdir() failed: %s", err.Error())
			}
		case tar.TypeReg:
			dir, _ := filepath.Split(p)
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				if err := os.MkdirAll(dir, modeDir); err != nil {
					return fmt.Errorf("ExtractTarGz: MkdirAll() failed: %s", err.Error())
				}
			}
			outFile, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE|os.O_TRUNC, fs.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("ExtractTarGz: OpenFile() failed: %s", err.Error())
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				return fmt.Errorf("ExtractTarGz: Copy() failed: %s", err.Error())
			}
			outFile.Close()

		default:
			return fmt.Errorf(
				"ExtractTarGz: unknown type: %v in %s",
				header.Typeflag,
				header.Name)
		}
	}

	return nil
}
