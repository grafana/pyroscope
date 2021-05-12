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
	// TODO: better upper boundary, currently 50 is a magic number
	for i := 0; i < 50; i++ {
		durations = append(durations, d)
		d *= time.Duration(multiplier)
	}
}
