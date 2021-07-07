package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_defaultMaxFrameSize(t *testing.T) {
	assert.Equal(t, defaultMaxFrameSize(1234), int64(1234))
	assert.Equal(t, defaultMaxFrameSize(0), int64(DefaultMaxFrameSize))
}
