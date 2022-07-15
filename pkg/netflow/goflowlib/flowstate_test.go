package goflowlib

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/netflow/common"
)

func TestStartFlowRoutine_invalidType(t *testing.T) {
	state, err := StartFlowRoutine("invalid", "my-hostname", 1234, 1, "my-ns", make(chan *common.Flow))
	assert.EqualError(t, err, "unknown flow type: invalid")
	assert.Nil(t, state)
}
