package cli

import (
	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

// mergeTagsWithAppName validates user input and merges explicitly specified
// tags with tags from app name.
//
// App name may be in the full form including tags (app.name{foo=bar,baz=qux}).
// Returned application name is always short, any tags that were included are
// moved to tags map. When merged with explicitly provided tags (config/CLI),
// last take precedence.
//
// App name may be an empty string. Tags must not contain reserved keys,
// the map is modified in place.
func mergeTagsWithAppName(appName string, tags map[string]string) (string, error) {
	k, err := storage.ParseKey(appName)
	if err != nil {
		return "", err
	}
	appName = k.AppName()
	if tags == nil {
		return appName, nil
	}
	// Note that at this point k may contain
	// reserved tag keys (e.g. '__name__').
	for tagKey, v := range k.Labels() {
		if flameql.IsTagKeyReserved(tagKey) {
			continue
		}
		if _, ok := tags[tagKey]; !ok {
			tags[tagKey] = v
		}
	}
	for tagKey := range tags {
		if err = flameql.ValidateTagKey(tagKey); err != nil {
			return "", err
		}
	}
	return appName, nil
}
