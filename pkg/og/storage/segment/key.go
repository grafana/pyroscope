package segment

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/grafana/pyroscope/pkg/og/flameql"
	"github.com/grafana/pyroscope/pkg/og/structs/sortedmap"
)

// TODO(kolesnikovae):
//   Rename tags to labels
//   Segment key -> LabelSet
//   Segment key to be moved to /model package
//   FlameQL to be split.

type Key struct {
	labels map[string]string
}

type ParserState int

const (
	nameParserState ParserState = iota
	tagKeyParserState
	tagValueParserState
	doneParserState
)

func NewKey(labels map[string]string) *Key { return &Key{labels: labels} }

func ParseKey(name string) (*Key, error) {
	k := &Key{labels: make(map[string]string)}
	p := parserPool.Get().(*parser)
	defer parserPool.Put(p)
	p.reset()
	var err error
	for _, r := range name + "{" {
		switch p.parserState {
		case nameParserState:
			err = p.nameParserCase(r, k)
		case tagKeyParserState:
			p.tagKeyParserCase(r)
		case tagValueParserState:
			err = p.tagValueParserCase(r, k)
		}
		if err != nil {
			return nil, err
		}
	}
	return k, nil
}

func ValidateKey(k *Key) error {
	if k == nil {
		return flameql.ErrInvalidTagKey
	}

	for key, v := range k.labels {
		if key == "__name__" {
			if err := flameql.ValidateAppName(v); err != nil {
				return err
			}
		} else {
			if err := flameql.ValidateTagKey(key); err != nil {
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

// ParseKey's nameParserState switch case
func (p *parser) nameParserCase(r int32, k *Key) error {
	switch r {
	case '{':
		p.parserState = tagKeyParserState
		appName := strings.TrimSpace(p.value.String())
		if err := flameql.ValidateAppName(appName); err != nil {
			return err
		}
		k.labels["__name__"] = appName
	default:
		p.value.WriteRune(r)
	}
	return nil
}

// ParseKey's tagKeyParserState switch case
func (p *parser) tagKeyParserCase(r rune) {
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

// ParseKey's tagValueParserState switch case
func (p *parser) tagValueParserCase(r rune, k *Key) error {
	switch r {
	case ',', '}':
		p.parserState = tagKeyParserState
		key := strings.TrimSpace(p.key.String())
		if !flameql.IsTagKeyReserved(key) {
			if err := flameql.ValidateTagKey(key); err != nil {
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

func (k *Key) SegmentKey() string {
	return k.Normalized()
}

const ProfileIDLabelName = "profile_id"

func (k *Key) HasProfileID() bool {
	v, ok := k.labels[ProfileIDLabelName]
	return ok && v != ""
}

func (k *Key) ProfileID() (string, bool) {
	id, ok := k.labels[ProfileIDLabelName]
	return id, ok
}

func AppSegmentKey(appName string) string { return appName + "{}" }

func TreeKey(k string, depth int, unixTime int64) string {
	return k + ":" + strconv.Itoa(depth) + ":" + strconv.FormatInt(unixTime, 10)
}

func (k *Key) TreeKey(depth int, t time.Time) string {
	return TreeKey(k.Normalized(), depth, t.Unix())
}

var errKeyInvalid = errors.New("invalid key")

// ParseTreeKey retrieves tree time and depth level from the given key.
func ParseTreeKey(k string) (time.Time, int, error) {
	a := strings.Split(k, ":")
	if len(a) < 3 {
		return time.Time{}, 0, errKeyInvalid
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

func (k *Key) DictKey() string {
	return k.labels["__name__"]
}

// FromTreeToDictKey returns app name from tree key k: given tree key
// "foo{}:0:1234567890", the call returns "foo".
//
// Before tags support, segment key form (i.e. app name + tags: foo{key=value})
// has been used to reference a dictionary (trie).
func FromTreeToDictKey(k string) string {
	return k[0:strings.IndexAny(k, "{")]
}

func (k *Key) Normalized() string {
	var sb strings.Builder

	sortedMap := sortedmap.New()
	for k, v := range k.labels {
		if k == "__name__" {
			sb.WriteString(v)
		} else {
			sortedMap.Put(k, v)
		}
	}

	sb.WriteString("{")
	for i, k := range sortedMap.Keys() {
		v := sortedMap.Get(k).(string)
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

func (k *Key) Clone() *Key {
	newMap := make(map[string]string)
	for k, v := range k.labels {
		newMap[k] = v
	}
	return &Key{labels: newMap}
}

func (k *Key) AppName() string {
	return k.labels["__name__"]
}

func (k *Key) Labels() map[string]string {
	return k.labels
}

func (k *Key) Add(key, value string) {
	if value == "" {
		delete(k.labels, key)
	} else {
		k.labels[key] = value
	}
}

// Match reports whether the key matches the query.
func (k *Key) Match(q *flameql.Query) bool {
	if k.AppName() != q.AppName {
		return false
	}
	for _, m := range q.Matchers {
		var ok bool
		for labelKey, labelValue := range k.labels {
			if m.Key != labelKey {
				continue
			}
			if m.Match(labelValue) {
				if !m.IsNegation() {
					ok = true
					break
				}
			} else if m.IsNegation() {
				return false
			}
		}
		if !ok && !m.IsNegation() {
			return false
		}
	}
	return true
}
