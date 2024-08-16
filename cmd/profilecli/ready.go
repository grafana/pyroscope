package main

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
)

var (
	// Occurs when Pyroscope is not ready.
	notReadyErr = errors.New("not ready")
)

type readyParams struct {
	*phlareClient
}

func addReadyParams(cmd commander) *readyParams {
	params := &readyParams{}
	params.phlareClient = addPhlareClient(cmd)

	return params
}

func ready(ctx context.Context, params *readyParams) error {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/ready", params.URL), nil)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)

	client := params.phlareClient.httpClient()
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	bytes, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusOK {
		level.Error(logger).Log("msg", "not ready", "status", res.Status, "body", string(bytes))
		return notReadyErr
	}

	level.Info(logger).Log("msg", "ready")
	return nil
}
