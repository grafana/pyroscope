package client

import (
	"errors"
)

var ErrNotFound = errors.New("file not found")

type File struct {
	Content string
	URL     string
}

type FileRequest struct {
	Owner string
	Repo  string
	Path  string
	Ref   string
}
