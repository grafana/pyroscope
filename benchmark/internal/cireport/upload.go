package cireport

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"

	s3go "github.com/rlmcpherson/s3gof3r"
	"github.com/sirupsen/logrus"
)

type fs struct {
}

func NewFsWriter() *fs {
	return &fs{}
}

func (rs *fs) WriteFile(dest string, data []byte) (string, error) {
	if _, err := os.Stat(dest); err != nil && !os.IsNotExist(err) {
		// unkown error
		return "", err
	}

	if dest == "" {
		return "", fmt.Errorf("filename can't be null")
	}

	logrus.Debug("writing file ", dest)

	err := os.WriteFile(dest, data, 0666)
	if err != nil {
		return "", err
	}

	// not really useful since we don't have the full path
	if _, err := os.Stat(dest); err == nil {
		// file exists
	} else if os.IsNotExist(err) {
		// file does not exist
	} else {
		// unkown error
		return "", err
	}
	return "file://" + dest, nil
}

type s3Writer struct {
	bucket *s3go.Bucket
}

// NewS3Writer creates a s3
// it assumes AWS environment variables are setup correctly
func NewS3Writer(bucketName string) (*s3Writer, error) {
	if bucketName == "" {
		return nil, fmt.Errorf("bucket name can't be empty")
	}

	keys, err := s3go.EnvKeys()
	if err != nil {
		return nil, err
	}

	s3 := s3go.New("", keys)
	b := s3.Bucket(bucketName)

	return &s3Writer{
		bucket: b,
	}, nil
}

func (s3Writer *s3Writer) WriteFile(dest string, data []byte) (string, error) {
	if dest == "" {
		return "", fmt.Errorf("filename can't be null")
	}

	r := bytes.NewReader(data)
	w, err := s3Writer.bucket.PutWriter(dest, http.Header{
		"Content-Type": []string{http.DetectContentType(data)},
	}, nil)
	if err != nil {
		return "", err
	}

	if _, err = io.Copy(w, r); err != nil {
		return "", err
	}

	if err = w.Close(); err != nil {
		return "", err
	}

	return fmt.Sprintf("http://%s.s3.amazonaws.com/%s", s3Writer.bucket.Name, dest), nil
}
