package storage

import (
	"context"
	"fmt"

	"github.com/pyroscope-io/pyroscope/pkg/storage/dimension"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

type DeleteInput struct {
	// Key must match exactly one segment.
	Key *segment.Key
}

func (s *Storage) Delete(_ context.Context, di *DeleteInput) error {
	return s.deleteSegmentAndRelatedData(di.Key)
}

func (s *Storage) deleteSegmentAndRelatedData(k *segment.Key) error {
	sk := k.SegmentKey()

	// Drop trees from disk.
	if err := s.trees.DropPrefix(treePrefix.key(sk)); err != nil {
		return err
	}
	// Discarding cached items is necessary because otherwise
	// those would be written back to disk on eviction.
	s.trees.DiscardPrefix(sk)
	for key, value := range k.Labels() {
		d, ok := s.lookupDimensionKV(key, value)
		if !ok {
			continue
		}
		d.Delete(dimension.Key(sk))

		if len(d.Keys) > 0 {
			continue
		}
		// There are no more references.
		if err := s.labels.Delete(key, value); err != nil {
			return err
		}
		if key == "__name__" {
			if err := s.dicts.Delete(k.DictKey()); err != nil {
				return err
			}
		}
	}
	return s.segments.Delete(sk)
}

// DeleteApp fully deletes an app
// It does so by deleting Segments, Dictionaries, Trees, Dimensions and Labels
// It's an idempotent call, ie. if the app already does not exist, no error is triggered.
// TODO cancelation?
func (s *Storage) DeleteApp(_ context.Context, appname string) error {
	/***********************************/
	/*      V a l i d a t i o n s      */
	/***********************************/
	s.logger.Debugf("deleting app '%s' \n", appname)
	key, err := segment.ParseKey(appname)
	if err != nil {
		return err
	}

	// the only label expected is __name__
	s.logger.Debugf("found %d labels\n", len(key.Labels()))
	if len(key.Labels()) != 1 {
		return fmt.Errorf("only app name is supported")
	}

	s.logger.Debugf("looking for __name__ key\n")
	nameKey := "__name__"
	_, ok := key.Labels()[nameKey]
	if !ok {
		return fmt.Errorf("could not __name__ key")
	}

	/*****************************/
	/*      D e l e t i o n      */
	/*****************************/

	//  d is the dimension of the application. Its 'Keys' member lists all
	//  the segments of the application. Every segments describes a series –
	//  a unique combination of the label key-value pairs. These segments have
	//  to be removed.
	//
	//  For example, given the app name "my_application". It's dimension could
	//  be represented as follows (we assume that there were two app instances
	//  with distinct tag sets):
	//    __name__=my_application
	//      my_application{foo=bar,baz=qux}
	//      my_application{foo=bar,baz=wadlo}
	//
	//  Although as far as we drop the whole application, we could delete this
	//  dimension, we also have to take care of the dimensions affected by the app
	//  segments:
	//    my_application{foo=bar,baz=qux}
	//    my_application{foo=bar,baz=wadlo}
	//
	//  In the example these dimensions are 'foo:bar', 'baz:qux', 'baz:wadlo'.
	//  We have to remove app segment keys from all the associated dimensions
	//  and, if the dimension is not referenced anymore, remove it alongside
	//  which the label KV pair itself:
	//    foo=bar
	//      my_application{foo=bar,baz=qux}
	//      my_application{foo=bar,baz=wadlo}
	//    baz=qux
	//      my_application{foo=bar,baz=qux}
	//    baz=wadlo
	//      my_application{foo=bar,baz=wadlo}
	//
	//  As you can see, these dimensions (and labels) are to be removed, simply
	//  because there are no more segments/apps referencing the dimension.
	//  But let's say we have one more app:
	//    __name__=my_another_application
	//      my_another_application{foo=bar}
	//
	//  Thus, our 'foo=bar' dimension would look differently:
	//    foo=bar
	//      my_application{foo=bar,baz=qux}
	//      my_application{foo=bar,baz=wadlo}
	//      my_another_application{foo=bar}
	//
	// In this case, 'foo=bar' dimension (and the corresponding label KV pair)
	// must not be removed when we delete 'my_application' but should only
	// include 'my_another_application':
	//   foo=bar
	//     my_another_application{foo=bar}
	s.logger.Debugf("looking for app dimension '%s'\n", appname)
	d, ok := s.lookupAppDimension(appname)
	if !ok {
		// Technically this does not necessarily mean the dimension does not exist
		// Since this could be triggered by an error
		s.logger.Debugf("dimensions could not be found, exiting early")
		return nil
	}

	s.logger.Debugf("iterating over dimension keys (aka segments keys)\n")
	for _, segmentKey := range d.Keys {
		sk2, err := segment.ParseKey(string(segmentKey))
		if err != nil {
			return err
		}

		s.logger.Debugf("iterating over segment %s labels\n", segmentKey)
		for labelKey, labelValue := range sk2.Labels() {
			// skip __name__, since this is the dimension we are already iterating
			if labelKey == "__name__" {
				continue
			}

			s.logger.Debugf("looking up dimension with key='%s' and value='%s'\n", labelKey, labelValue)
			d2, ok := s.lookupDimensionKV(labelKey, labelValue)
			if !ok {
				s.logger.Debugf("skipping since dimension could not be found\n")
				continue
			}

			s.logger.Debugf("deleting dimension with key %s\n", segmentKey)
			d2.Delete(dimension.Key(segmentKey))

			// We can only delete the dimension once it's not pointing to any segments
			if len(d2.Keys) > 0 {
				s.logger.Debugf("dimension is still pointing to valid segments. not deleting it. \n")
				continue
			}

			s.logger.Debugf("deleting labels %s=%s \n", labelKey, labelValue)
			if err := s.labels.Delete(labelKey, labelValue); err != nil {
				return err
			}

			s.logger.Debugf("deleting dimension %s=%s \n", labelKey, labelValue)
			if err := s.dimensions.Delete(labelKey + ":" + labelValue); err != nil {
				return err
			}
		}
	}

	appWithCurlyBrackets := appname + "{"

	s.logger.Debugf("deleting trees with prefix %s\n", appWithCurlyBrackets)
	if err = s.trees.DiscardPrefix(appWithCurlyBrackets); err != nil {
		return err
	}

	s.logger.Debugf("deleting segments with prefix %s\n", appWithCurlyBrackets)
	if err = s.segments.DiscardPrefix(appWithCurlyBrackets); err != nil {
		return err
	}

	s.logger.Debugf("deleting dicts %s\n", key.DictKey())
	if err := s.dicts.Delete(key.DictKey()); err != nil {
		return err
	}

	s.logger.Debugf("deleting labels\n")
	if err := s.labels.Delete("__name__", appname); err != nil {
		return err
	}

	s.logger.Debugf("deleting dimensions for __name__=%s\n", appname)
	return s.dimensions.Delete("__name__:" + appname)
}
