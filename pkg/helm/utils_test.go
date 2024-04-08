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

func TestDictionaryLoading(t *testing.T) {
	t.Run("AdjectiveList", func(t *testing.T) {
		list, err := loadAdjectiveList()
		assert.NoError(t, err)
		assert.Len(t, list, 1202)
	})
	t.Run("AnimalList", func(t *testing.T) {
		list, err := loadAnimalList()
		assert.NoError(t, err)
		assert.Len(t, list, 355)
	})
}

func TestRandomNameGeneration(t *testing.T) {
	t.Run("RandomName", func(t *testing.T) {
		var fixedSeed int64 = 1337
		name, err := generateRandomName(&fixedSeed)
		assert.NoError(t, err)
		assert.Equal(t, "excessive-roadrunner", name)
	})
}
