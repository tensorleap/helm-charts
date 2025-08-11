package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateTensorleapChartValues(t *testing.T) {
	t.Run("CreateTensorleapChartValues", func(t *testing.T) {
		params := &ServerHelmValuesParams{
			Gpu:                   true,
			LocalDataDirectories:  []string{"some/dir/path"},
			DisableDatadogMetrics: false,
			Domain:                "",
			Url:                   "",
			Tls: TLSParams{
				Enabled: false,
				Cert:    "",
				Key:     "",
			},
			HostName:        "nsa.gov",
			KeycloakEnabled: true,
		}

		expected := Record{
			"tensorleap-engine": Record{
				"gpu":                  params.Gpu,
				"localDataDirectories": params.LocalDataDirectories,
			},
			"tensorleap-node-server": Record{
				"enableKeycloak":        params.KeycloakEnabled,
				"disableDatadogMetrics": params.DisableDatadogMetrics,
			},
			"global": Record{
				"domain":               "",
				"url":                  "",
				"proxyUrl":             "",
				"basePath":             "",
				"create_local_volumes": true,
				"storageClassName":     "",
				"keycloak": Record{
					"enabled": true,
				},
				"tls": Record{
					"enabled": false,
					"cert":    "",
					"key":     "",
				},
			},
			"datadog": Record{
				"enabled": !params.DisableDatadogMetrics,
				"datadog": Record{
					"env": []map[string]string{
						{
							"name":  "DD_HOSTNAME",
							"value": params.HostName,
						},
					},
				},
			},
			"keycloak": map[string]interface{}{
				"enabled":  true,
				"replicas": 1,
				"extraEnv": "\n- name: KEYCLOAK_USER\n  value: admin\n- name: KEYCLOAK_PASSWORD\n  value: admin\n- name: PROXY_ADDRESS_FORWARDING\n  value: \"true\"\n",
			},
		}

		result, err := CreateTensorleapChartValues(params)
		assert.NoError(t, err)

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
