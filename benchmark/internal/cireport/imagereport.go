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
)

type uploader interface {
	WriteFile(dest string, data []byte) (string, error)
}

type imageReporter struct {
	grafanaURL     string
	timeoutSeconds int

	uploader uploader
}

func NewImageReporter(grafanaURL string, timeoutSeconds int, uploadType string, uploadDestPath string) (*imageReporter, error) {
	var uploader uploader
	switch uploadType {
	case "s3":
		u, err := NewS3Writer(uploadDestPath)
		uploader = u

		if err != nil {
			return nil, err
		}
	case "fs":
		uploader = NewFsWriter()
	default:
		return nil, fmt.Errorf("invalid upload type: '%s'", uploadType)
	}

	return &imageReporter{
		grafanaURL,
		timeoutSeconds,
		uploader,
	}, nil
}

type screenshotPanel struct {
	Title string
	Url   string
}

func (r *imageReporter) ImageReport(ctx context.Context, dashboardUID string, dir string, from int64, to int64) (string, error) {

	gs := GrafanaScreenshotter{
		GrafanaURL:     r.grafanaURL,
		TimeoutSeconds: r.timeoutSeconds,
	}

	// screenshot all panes
	logrus.Debug("taking screenshot of all panels")
	panels, err := gs.AllPanels(ctx, dashboardUID, from, to)
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
			publicUrl, err := r.uploader.WriteFile(filename(dir, p.Title), p.Data)
			if err != nil {
				return err
			}

			// TODO lock this?
			sp[i].Title = p.Title
			sp[i].Url = publicUrl
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return "", err
	}

	logrus.Debug("generating markdown report")
	return r.template(sp)
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

func (r *imageReporter) template(panels []screenshotPanel) (string, error) {
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
