package cireport

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/sirupsen/logrus"
)

type MetaReport struct {
	allowlist map[string]bool
}

// NewMetaReport creates a meta report
// the allowlist parameter refers to which keys are valid
func NewMetaReport(allowlist []string) (*MetaReport, error) {
	if len(allowlist) <= 0 {
		return nil, fmt.Errorf("at least one item should be allowed")
	}

	a := make(map[string]bool)
	for _, v := range allowlist {
		a[v] = true
	}

	return &MetaReport{
		allowlist: a,
	}, nil
}

type meta struct {
	Key string
	Val string
}

// Report generates a markdown report
func (mr *MetaReport) Report(vars []string) (string, error) {
	if len(vars) <= 0 {
		return "", fmt.Errorf("at least one item should be reported")
	}

	// transform 'A=B' into {key: A, val: B}
	m := make([]meta, 0, len(vars))
	for _, v := range vars {
		key, val, err := mr.breakOnEqual(v)
		logrus.Debugf("breaking string %s on '=' produces key %s value %s\n", v, key, val)
		if err != nil {
			return "", err
		}

		n := meta{key, val}
		m = append(m, n)
	}

	// validate it's in the allowlist
	logrus.Debug("validating there're no values not in the allowlist")
	err := mr.validate(m)
	if err != nil {
		return "", err
	}

	logrus.Debug("generating template")
	report, err := mr.tpl(m)
	if err != nil {
		return "", err
	}

	return report, nil
}

func (*MetaReport) breakOnEqual(s string) (key string, value string, err error) {
	split := strings.Split(s, "=")
	if len(split) != 2 {
		return "", "", fmt.Errorf("expect value in format A=B")
	}

	if split[0] == "" || split[1] == "" {
		return "", "", fmt.Errorf("expect non empty key/value")
	}

	return split[0], split[1], nil
}

func (mr *MetaReport) validate(m []meta) error {
	for _, v := range m {
		if _, ok := mr.allowlist[v.Key]; !ok {
			return fmt.Errorf("key is not allowed: '%s'", v.Key)
		}
	}

	return nil
}

func (*MetaReport) tpl(m []meta) (string, error) {
	var tpl bytes.Buffer

	data := struct {
		Meta []meta
	}{
		Meta: m,
	}
	t, err := template.New("meta-report.gotpl").
		ParseFS(resources, "resources/meta-report.gotpl")

	if err != nil {
		return "", err
	}

	if err := t.Execute(&tpl, data); err != nil {
		return "", err
	}

	return tpl.String(), nil
}
