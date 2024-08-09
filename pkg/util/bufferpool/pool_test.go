package bufferpool

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_returnPool(t *testing.T) {
	assert.EqualValues(t, 0, returnPool(512, 0)) // Buffers can be added to the pool.
	assert.EqualValues(t, 1, returnPool(513, 0))
	assert.EqualValues(t, 1, returnPool(1<<10, 0))
	assert.EqualValues(t, -1, returnPool(0, 0))    // Empty buffers are ignored.
	assert.EqualValues(t, -1, returnPool(0, 10))   //
	assert.EqualValues(t, 5, returnPool(1<<14, 0)) // New buffers are added to the appropriate pool.
	assert.EqualValues(t, 5, returnPool(1<<14, 3)) // Buffer of a capacity exceeding the next power of two are relocated.
	assert.EqualValues(t, 4, returnPool(1<<14, 4)) // Buffer of a capacity not exceeding the next power of two are retained.
	assert.EqualValues(t, 5, returnPool(1<<14, 5)) // Buffer of the nominal capacity.
	assert.EqualValues(t, 5, returnPool(1<<14, 6)) // Buffer of a smaller capacity must be relocated.
	assert.EqualValues(t, 21, returnPool(1<<30, 13))
	assert.EqualValues(t, -1, returnPool(1<<30+1, 13)) // No pools for buffers larger than 4MB.
}
