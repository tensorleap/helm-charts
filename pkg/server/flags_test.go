package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalcEndpointUrl(t *testing.T) {
	flags := &InstallFlags{
		EndpointUrl: "",
		Port:        8080,
	}
	result := flags.CalcEndpointUrl()
	assert.Equal(t, "http://localhost:8080", result)

	flags = &InstallFlags{
		EndpointUrl: "http://leap.ai",
		Port:        8080,
	}
	result = flags.CalcEndpointUrl()
	assert.Equal(t, "http://leap.ai", result)
}
