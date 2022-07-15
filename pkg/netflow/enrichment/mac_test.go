package enrichment

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatMacAddress(t *testing.T) {
	assert.Equal(t, "82:a5:6e:a5:aa:99", FormatMacAddress(uint64(143647037565593)))
	assert.Equal(t, "00:00:00:00:00:00", FormatMacAddress(uint64(0)))
}
