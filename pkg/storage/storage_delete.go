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
//
// TODO: make this a test?
// To make it concrete, the comments will use as an example:
// Key: 'simple.golang.app2.cpu'
//
// Dimensions:
// i:__name__:simple.golang.app2.cpu =>
//			-simple.golang.app2.cpu{foo=bar,function=fast} (segmentKey)
//			-simple.golang.app2.cpu{foo=bar,function=slow}
//			simple.golang.app2.cpu{foo=bar}
//			simple.golang.app2.cpu{}
// i:function:fast =>
//		,simple.golang.app.cpu{foo=bar,function=fast}
//		-simple.golang.app2.cpu{foo=bar,function=fast}
// i:function:slow
//
// Trees:
// t:simple.golang.app2.cpu{foo=bar,function=fast}:0:1637611090
// t:simple.golang.app2.cpu{foo=bar,function=fast}:0:1637611100
// t:simple.golang.app2.cpu{foo=bar,function=fast}:0:1637626800
// t:simple.golang.app2.cpu{foo=bar,function=fast}:0:1637626900
// t:simple.golang.app2.cpu{foo=bar,function=fast}:0:1637626920
// t:simple.golang.app2.cpu{foo=bar,function=fast}:1:1637626900
// t:simple.golang.app2.cpu{foo=bar,function=fast}:2:1637610200
// t:simple.golang.app2.cpu{foo=bar,function=fast}:2:1637626200
// t:simple.golang.app2.cpu{foo=bar,function=fast}:4:1637603200
// t:simple.golang.app2.cpu{foo=bar,function=slow}:0:1637611090
// t:simple.golang.app2.cpu{foo=bar,function=slow}:0:1637611100
// t:simple.golang.app2.cpu{foo=bar,function=slow}:0:1637626800
// t:simple.golang.app2.cpu{foo=bar,function=slow}:0:1637626900
// t:simple.golang.app2.cpu{foo=bar,function=slow}:0:1637626920
// t:simple.golang.app2.cpu{foo=bar,function=slow}:1:1637626900
// t:simple.golang.app2.cpu{foo=bar,function=slow}:2:1637610200
// t:simple.golang.app2.cpu{foo=bar,function=slow}:2:1637626200
// t:simple.golang.app2.cpu{foo=bar,function=slow}:4:1637603200
// t:simple.golang.app2.cpu{foo=bar}:0:1637626800
// t:simple.golang.app2.cpu{}:0:1637626900
// t:simple.golang.app2.cpu{}:0:1637626920
// t:simple.golang.app2.cpu{}:1:1637626900
//
// Dictionaries:
// d:simple.golang.app2.cpu
//
// Segments:
// s:simple.golang.app2.cpu{foo=bar,function=fast}
// s:simple.golang.app2.cpu{foo=bar,function=slow}
// s:simple.golang.app2.cpu{foo=bar}
// s:simple.golang.app2.cpu{}

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
	prefix := treePrefix.key(appname + "{")
	s.logger.Debugf("dropping from DISK all trees with prefix '%s'\n", prefix)
	if err := s.trees.DropPrefix(prefix); err != nil {
		return err
	}

	// Discarding cached items is necessary because otherwise
	// those would be written back to disk on eviction.
	s.logger.Debugf("dropping from CACHE all trees with prefix '%s'\n", appname+"{")
	s.trees.DiscardPrefix(appname + "{")

	// DO THE SAME THING FOR SEGMENTS
	// Discarding cached items is necessary because otherwise
	// those would be written back to disk on eviction.
	s.logger.Debugf("dropping from DISK all segments with prefix '%s'\n", prefix)
	if err := s.segments.DropPrefix(segmentPrefix.key(appname + "{")); err != nil {
		return err
	}
	s.logger.Debugf("dropping from CACHE all segments with prefix '%s'\n", appname+"{")
	s.segments.DiscardPrefix(appname + "{")

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
		// TODO
		// this doesn't mean the storage doesn't exist
		// it just means it failed for some reason
		return fmt.Errorf("dimensions don't exist")
	}

	s.dimensions.Delete("__name__:" + appname)

	err = s.segments.DropPrefix(segmentPrefix.key(appname + "{"))
	if err != nil {
		return err
	}

	// delete all dimensions our __name__ was pointing to
	// eg
	// "simple.golang.app2.cpu{foo=bar,function=fast}"
	// "simple.golang.app2.cpu{foo=bar,function=slow}"
	s.logger.Debugf("deleting all dimensions pointed by '%s:%s'", nameKey, value)
	for _, segmentKey := range d.Keys {

		s.logger.Debug("segmentKey", segmentKey)
		// segmentKey: simple.golang.app2.cpu{foo=bar,function=fast}
		//		s.logger.Debugf("deleting dimension %s", myValue)

		// sk2 is
		//   map[string]string [
		//     "__name__": "my.app.cpu",
		//     "foo": "bar",
		//     "function": "fast",
		//  ],}
		sk2, err := segment.ParseKey(string(segmentKey))
		if err != nil {
			return err
		}

		fmt.Println("segmentKey", string(segmentKey))
		//		s.dimensions.Delete(labelKey + ":" + labelValue)

		// iterate over the labels
		// ["__name__": "my.app.cpu", "foo": "bar", "function": "fast"]
		for labelKey, labelValue := range sk2.Labels() {
			// let's use 'function=fast' as an example
			s.logger.Debugf("labelKey %s\n", labelKey)
			s.logger.Debugf("labelValue %s\n", labelValue)

			fmt.Println("labelKey", labelKey)
			fmt.Println("labelValue", labelValue)

			// function:fast should still exist
			d2, ok := s.lookupDimensionKV(labelKey, labelValue)
			if !ok {
				continue
			}

			// d2.Delete(dimension.Key(sk))
			//			s.logger.Debugf("deleting segmentKey %s\n", segmentKey)
			//			d2.Delete(segmentKey)

			// there are no more keys
			// so we can remove this dimension

			//			fmt.Println("for key", string(segmentKey))
			//			for _, b := range d2.Keys {
			//				fmt.Println("there's key", string(b))
			//			}

			// if function:fast is pointing to something that still exists
			// that means we can't delete it yet
			// TODO i think we gonna have to add another look up

			// TODO
			// check if all the keys are still pointing to something that exists

			found := false
			for _, k := range d2.Keys {
				_, ok := s.segments.Lookup(string(k))
				if ok {
					found = true
				} else {
					// that key is pointing to something not valid
					d2.Delete(k)
				}
			}

			// there are still keys pointing to valid segments
			// which means we can't delete it yet
			if found {
				continue
			}
			//			if len(d2.Keys) > 0 {
			//				continue
			//			}

			// THIS DOESNT EVEN SEEM TO BE CALLED
			//			s.dimensions.Delete(key)
			s.dimensions.Delete(labelKey + ":" + labelValue)

			// there's no cache here
			if err := s.labels.Delete(labelKey, labelValue); err != nil {
				return err
			}
		}

		// There are no more references.
		//		d.Delete(myValue)
	}

	s.logger.Debug("deleting dimension", "__name__"+":"+appname)
	s.dimensions.Delete("__name__" + ":" + appname)

	// delete the dictionary? TODO
	s.logger.Debugf("deleting dictionary '%s'", key.DictKey())
	if err := s.dicts.Delete(key.DictKey()); err != nil {
		return err
	}

	if err := s.labels.Delete("__name__", appname); err != nil {
		return err
	}

	// delete labels? TODO
	//	s.logger.Debugf("deleting labels '%s:%s'", nameKey, value)
	//	if err := s.labels.Delete(nameKey, value); err != nil {
	//		return err
	//	}

	//
	// TODO what about dimensions such as
	// i:foo:bar
	// i:function:fast
	// i:function:slow

	// TODO
	// delete segments
	// s:simple.golang.app2.cpu{foo=bar,function=fast}
	// s:simple.golang.app2.cpu{foo=bar,function=slow}
	// s:simple.golang.app2.cpu{foo=bar}

	//	}
	// TODO
	// s:simple.golang.app2.cpu{foo=bar,function=fast}
	// s:simple.golang.app2.cpu{foo=bar,function=slow}
	// s:simple.golang.app2.cpu{foo=bar}
	// s:simple.golang.app2.cpu{}
	//	return s.segments.Delete(sk)

	// TODO do this after
	// s.dimensions.Delete(nameKey + ":" + value)

	return nil
}
