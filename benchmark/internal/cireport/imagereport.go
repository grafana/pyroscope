package cireport

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unicode"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/pyroscope-io/pyroscope/benchmark/internal/config"
)

type Uploader interface {
	WriteFile(dest string, data []byte) (string, error)
}

type DashboardScreenshotter interface {
	AllPanels(ctx context.Context, dashboardUID string, from int64, to int64) ([]Panel, error)
}

type ImageReporter struct {
	uploader      Uploader
	screenshotter DashboardScreenshotter
}

func ImageReportCLI(cfg config.ImageReport) (string, error) {
	uploader, err := decideUploader(cfg.UploadType, cfg.UploadBucket)
	if err != nil {
		return "", err
	}

	gs := GrafanaScreenshotter{
		GrafanaURL:     cfg.GrafanaAddress,
		TimeoutSeconds: cfg.TimeoutSeconds,
	}

	r := NewImageReporter(gs, uploader)

	from, to := decideTimestamp(cfg.From, cfg.To)

	return r.Report(
		context.Background(),
		cfg.DashboardUID,
		cfg.UploadDest,
		from,
		to,
	)
}

type screenshotPanel struct {
	Title string
	URL   string
}

func NewImageReporter(screenshotter GrafanaScreenshotter, uploader Uploader) *ImageReporter {
	return &ImageReporter{
		uploader,
		&screenshotter,
	}
}

func (r *ImageReporter) Report(ctx context.Context, dashboardUID string, dir string, from int64, to int64) (string, error) {
	// screenshot all panes
	logrus.Debug("taking screenshot of all panels")
	panels, err := r.screenshotter.AllPanels(ctx, dashboardUID, from, to)
	if err != nil {
		return "", err
	}

	sp := make([]screenshotPanel, len(panels))

	// upload
	logrus.Debug("uploading screenshots")
	g, ctx := errgroup.WithContext(ctx)
	for i, p := range panels {
		p := p
		i := i

		g.Go(func() error {
			publicURL, err := r.uploader.WriteFile(filename(dir, p.Title), p.Data)
			if err != nil {
				return err
			}

			// TODO lock this?
			sp[i].Title = p.Title
			sp[i].URL = publicURL
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return "", err
	}

	logrus.Debug("generating markdown report")
	return r.tpl(sp)
}

func filename(dir string, s string) string {
	return path.Join(dir, normalizeWord(s)+".png")
}

func normalizeWord(s string) string {
	isMn := func(r rune) bool {
		return unicode.Is(unicode.Mn, r)
	}

	// unicode -> ascii
	t := transform.Chain(norm.NFD, transform.RemoveFunc(isMn), norm.NFC)
	// TODO: handle error
	result, _, _ := transform.String(t, s)

	result = strings.ReplaceAll(result, " ", "_")
	// TODO: handle error
	reg, _ := regexp.Compile("[^a-zA-Z0-9_]+")
	result = reg.ReplaceAllString(result, "")

	result = strings.ToLower(result)

	return result
}

func (*ImageReporter) tpl(panels []screenshotPanel) (string, error) {
	var tpl bytes.Buffer

	data := struct {
		Panels []screenshotPanel
		Ts     string
	}{
		Panels: panels,
		// cache bust
		Ts: strconv.FormatInt(time.Now().Unix(), 10),
	}
	t, err := template.New("image-report.gotpl").
		Funcs(template.FuncMap{}).
		ParseFS(resources, "resources/image-report.gotpl")
	if err != nil {
		return "", err
	}

	if err := t.Execute(&tpl, data); err != nil {
		return "", err
	}

	return tpl.String(), nil
}

func decideTimestamp(fromInt, toInt int) (from int64, to int64) {
	now := time.Now()
	from = int64(fromInt)
	to = int64(toInt)

	// set defaults if appropriate
	if to == 0 {
		// TODO use UnixMilli()
		to = now.UnixNano() / int64(time.Millisecond)
	}

	if from == 0 {
		// TODO use UnixMilli()
		from = now.Add(time.Duration(5)*-time.Minute).UnixNano() / int64(time.Millisecond)
	}

	return from, to
}

func decideUploader(uploadType string, uploadBucket string) (Uploader, error) {
	var uploader Uploader
	switch uploadType {
	case "s3":
		u, err := NewS3Writer(uploadBucket)
		uploader = u

		if err != nil {
			return nil, err
		}
	case "fs":
		uploader = &FsWriter{}
	default:
		return nil, fmt.Errorf("invalid upload type: '%s'", uploadType)
	}

	return uploader, nil
}
