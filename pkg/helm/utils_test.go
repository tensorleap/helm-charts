package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateTensorleapChartValues(t *testing.T) {
	t.Run("CreateTensorleapChartValues", func(t *testing.T) {
		params := &ServerHelmValuesParams{
			Gpu:                   true,
			LocalDataDirectory:    "some/dir/path",
			DisableDatadogMetrics: false,
			Domain:                "",
			Url:                   "",
			Tls: TLSParams{
				Enabled: false,
				Cert:    "",
				Key:     "",
			},
		}

		expected := Record{
			"tensorleap-engine": Record{
				"gpu":                params.Gpu,
				"localDataDirectory": params.LocalDataDirectory,
			},
			"tensorleap-node-server": Record{
				"disableDatadogMetrics": params.DisableDatadogMetrics,
			},
			"global": Record{
				"domain": "",
				"url":    "",
				"tls": Record{
					"enabled": false,
					"cert":    "",
					"key":     "",
				},
			},
		}

		result := CreateTensorleapChartValues(params)

		assert.Equal(t, expected, result)
	})
}
