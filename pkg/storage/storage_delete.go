package storage

import (
	"fmt"

	"github.com/pyroscope-io/pyroscope/pkg/storage/dimension"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

type DeleteInput struct {
	// Key must match exactly one segment.
	Key *segment.Key
}

func (s *Storage) Delete(di *DeleteInput) error {
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
// That is, it deletes deletes Segment, Dictionary and Trees
// And also deletes Dimensions and Labels if appropriate,
// IE when the references do not exist
func (s *Storage) DeleteApp(appname string) error {
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
	value, ok := key.Labels()[nameKey]
	if !ok {
		return fmt.Errorf("could not required find app name")
	}

	// Invariants
	// From down below we know:
	// That there's only one label, '__name__'

	/*****************************/
	/*      D e l e t i o n      */
	/*****************************/

	// TODO:
	// DELETE TREES ONLY AFTER
	if err = s.trees.DiscardPrefix(appname + "{"); err != nil {
		return err
	}

	if err = s.segments.DiscardPrefix(appname + "{"); err != nil {
		return err
	}

	// Delete the name dimension
	// TODO what about the other dimensions?
	// for example
	// i:foo:bar
	// i:function:fast
	// i:function:slow
	// we have to figure out if they have
	s.logger.Debugf("looking up dimensions pointed by '%s:%s'\n", nameKey, value)

	//	s.dimensions.Delete(dimension.Key())

	// TODO
	// this doesn't seem correct?
	// it looks like i need to prefix with __name__
	//	s.logger.Debugf("deleting dimension '%s'\n", dimension.Key(sk))
	// this is deleting that key
	//d.Delete(dimension.Key(sk))

	// TODO save back to the db?

	// TODO
	// not working
	//s.logger.Debugf("deleting dimension '__name__:%s'\n", appname)
	//d.Delete(dimension.Key("__name__:" + appname))

	// TODO maybe this namekey is wrong?
	// i think i need to pass i: ?
	// BTW TODO what does i: stand for?
	//d, ok := s.lookupDimensionKV(nameKey, value)
	s.logger.Debug("appname ", appname)
	d, ok := s.lookupAppDimension(appname)
	if !ok {
		// TODO(eh-am):
		// technically this does not necessarily mean the dimension does not exist
		// since this could be triggered by an error
		s.logger.Debug("dimensions could not be found, exiting early")
		return nil
	}

	// TODO(kolesnikovae):
	//
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
	//

	// TODO(kolesnikovae): Too early. If the process was canceled (do we need to add
	//  support for this?) or if the process was killed for example, we may not be able
	//  to lookup all the data (associated dimensions and labels first of all.)

	//fmt.Println("---")
	//s.dimensions.Dump()
	//fmt.Println("---")
	//	s.dimensions.Delete("__name__:" + appname)
	//s.dimensions.Dump()
	//fmt.Println("---")

	//	err = s.segments.DropPrefix(segmentPrefix.key(appname + "{"))
	//	if err != nil {
	//		return err
	//	}

	// delete all dimensions our __name__ was pointing to
	// eg
	// "simple.golang.app2.cpu{foo=bar,function=fast}"
	// "simple.golang.app2.cpu{foo=bar,function=slow}"
	s.logger.Debugf("deleting all dimensions pointed by '%s:%s'", nameKey, value)

	for _, segmentKey := range d.Keys {
		fmt.Println("--")
		fmt.Println("segmentKey", string(segmentKey))

		sk2, err := segment.ParseKey(string(segmentKey))
		if err != nil {
			return err
		}

		for labelKey, labelValue := range sk2.Labels() {
			// skip __name__, since this is the dimension we are already iterating
			if labelKey == "__name__" {
				continue
			}

			d2, ok := s.lookupDimensionKV(labelKey, labelValue)
			if !ok {
				continue
			}

			d2.Delete(dimension.Key(segmentKey))
			if len(d2.Keys) > 0 {
				continue
			}

			if err := s.labels.Delete(labelKey, labelValue); err != nil {
				return err
			}
			if err := s.dimensions.Delete(labelKey + ":" + labelValue); err != nil {
				return err
			}
		}
	}

	if err := s.dicts.Delete(key.DictKey()); err != nil {
		return err
	}
	if err := s.labels.Delete("__name__", appname); err != nil {
		return err
	}
	s.dimensions.Delete("__name__:" + appname)

	return nil
}
