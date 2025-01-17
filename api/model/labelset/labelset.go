// Package labelset implements parsing/normalizing of string representation of the Label Set of a profile.
//
// This is used by the /ingest endpoint, as described in the original Pyroscope API. We keep this mainly for backwards compatability.
package labelset

import (
	"bytes"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type LabelSet struct {
	labels map[string]string
}

type ParserState int

const (
	nameParserState ParserState = iota
	tagLabelSetParserState
	tagValueParserState
	doneParserState
)

func New(labels map[string]string) *LabelSet { return &LabelSet{labels: labels} }

func Parse(name string) (*LabelSet, error) {
	k := &LabelSet{labels: make(map[string]string)}
	p := parserPool.Get().(*parser)
	defer parserPool.Put(p)
	p.reset()
	var err error
	for _, r := range name + "{" {
		switch p.parserState {
		case nameParserState:
			err = p.nameParserCase(r, k)
		case tagLabelSetParserState:
			p.tagLabelSetParserCase(r)
		case tagValueParserState:
			err = p.tagValueParserCase(r, k)
		}
		if err != nil {
			return nil, err
		}
	}
	return k, nil
}

func ValidateLabelSet(k *LabelSet) error {
	if k == nil {
		return ErrInvalidLabelSet
	}

	for key, v := range k.labels {
		if key == ReservedLabelNameName {
			if err := ValidateServiceName(v); err != nil {
				return err
			}
		} else {
			if err := ValidateLabelName(key); err != nil {
				return err
			}
		}
	}
	return nil
}

type parser struct {
	parserState ParserState
	key         *bytes.Buffer
	value       *bytes.Buffer
}

var parserPool = sync.Pool{
	New: func() any {
		return &parser{
			parserState: nameParserState,
			key:         new(bytes.Buffer),
			value:       new(bytes.Buffer),
		}
	},
}

func (p *parser) reset() {
	p.parserState = nameParserState
	p.key.Reset()
	p.value.Reset()
}

// Parse's nameParserState switch case
func (p *parser) nameParserCase(r int32, k *LabelSet) error {
	switch r {
	case '{':
		p.parserState = tagLabelSetParserState
		serviceName := strings.TrimSpace(p.value.String())
		if err := ValidateServiceName(serviceName); err != nil {
			return err
		}
		k.labels["__name__"] = serviceName
	default:
		p.value.WriteRune(r)
	}
	return nil
}

// Parse's tagLabelSetParserState switch case
func (p *parser) tagLabelSetParserCase(r rune) {
	switch r {
	case '}':
		p.parserState = doneParserState
	case '=':
		p.parserState = tagValueParserState
		p.value.Reset()
	default:
		p.key.WriteRune(r)
	}
}

// Parse's tagValueParserState switch case
func (p *parser) tagValueParserCase(r rune, k *LabelSet) error {
	switch r {
	case ',', '}':
		p.parserState = tagLabelSetParserState
		key := strings.TrimSpace(p.key.String())
		if !IsLabelNameReserved(key) {
			if err := ValidateLabelName(key); err != nil {
				return err
			}
		}
		k.labels[key] = strings.TrimSpace(p.value.String())
		p.key.Reset()
	default:
		p.value.WriteRune(r)
	}
	return nil
}

func (k *LabelSet) LabelSet() string {
	return k.Normalized()
}

const ProfileIDLabelName = "profile_id"

func (k *LabelSet) HasProfileID() bool {
	v, ok := k.labels[ProfileIDLabelName]
	return ok && v != ""
}

func (k *LabelSet) ProfileID() (string, bool) {
	id, ok := k.labels[ProfileIDLabelName]
	return id, ok
}

func ServiceNameLabelSet(appName string) string { return appName + "{}" }

func TreeLabelSet(k string, depth int, unixTime int64) string {
	return k + ":" + strconv.Itoa(depth) + ":" + strconv.FormatInt(unixTime, 10)
}

func (k *LabelSet) TreeLabelSet(depth int, t time.Time) string {
	return TreeLabelSet(k.Normalized(), depth, t.Unix())
}

var errLabelSetInvalid = errors.New("invalid key")

// ParseTreeLabelSet retrieves tree time and depth level from the given key.
func ParseTreeLabelSet(k string) (time.Time, int, error) {
	a := strings.Split(k, ":")
	if len(a) < 3 {
		return time.Time{}, 0, errLabelSetInvalid
	}
	level, err := strconv.Atoi(a[1])
	if err != nil {
		return time.Time{}, 0, err
	}
	v, err := strconv.Atoi(a[2])
	if err != nil {
		return time.Time{}, 0, err
	}
	return time.Unix(int64(v), 0), level, err
}

func (k *LabelSet) DictLabelSet() string {
	return k.labels[ReservedLabelNameName]
}

// FromTreeToDictLabelSet returns app name from tree key k: given tree key
// "foo{}:0:1234567890", the call returns "foo".
//
// Before tags support, segment key form (i.e. app name + tags: foo{key=value})
// has been used to reference a dictionary (trie).
func FromTreeToDictLabelSet(k string) string {
	return k[0:strings.IndexAny(k, "{")]
}

func (l *LabelSet) Normalized() string {
	var sb strings.Builder

	labelNames := make([]string, 0, len(l.labels))
	for k, v := range l.labels {
		if k == ReservedLabelNameName {
			sb.WriteString(v)
		} else {
			labelNames = append(labelNames, k)
		}
	}

	sort.Slice(labelNames, func(i, j int) bool {
		return labelNames[i] < labelNames[j]
	})

	sb.WriteString("{")
	for i, k := range labelNames {
		v := l.labels[k]
		if i != 0 {
			sb.WriteString(",")
		}
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(v)
	}
	sb.WriteString("}")

	return sb.String()
}

func (k *LabelSet) Clone() *LabelSet {
	newMap := make(map[string]string)
	for k, v := range k.labels {
		newMap[k] = v
	}
	return &LabelSet{labels: newMap}
}

func (k *LabelSet) ServiceName() string {
	return k.labels[ReservedLabelNameName]
}

func (k *LabelSet) Labels() map[string]string {
	return k.labels
}

func (k *LabelSet) Add(key, value string) {
	if value == "" {
		delete(k.labels, key)
	} else {
		k.labels[key] = value
	}
}
