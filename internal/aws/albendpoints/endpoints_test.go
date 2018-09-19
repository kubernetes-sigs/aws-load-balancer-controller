package albendpoints

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsWAFRegionalAvailable(t *testing.T) {
	assert.Equal(t, true, isWAFRegionalAvailable("us-west-1"))
	assert.Equal(t, false, isWAFRegionalAvailable("eu-west-3"))
	assert.Equal(t, false, isWAFRegionalAvailable("foobar"))
}
