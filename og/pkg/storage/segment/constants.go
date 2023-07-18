package segment

import "time"

// TODO: at some point we should change it so that segments can support different
// resolution and multiplier values. For now they are constants
const (
	multiplier = 10
	resolution = 10 * time.Second
)

var durations = []time.Duration{}

func init() {
	d := resolution
	for i := 0; i < 50; i++ {
		durations = append(durations, d)
		newD := d * time.Duration(multiplier)
		if newD < d {
			return
		}
		d = newD
	}
}
