package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateTensorleapChartValues(t *testing.T) {
	t.Run("CreateTensorleapChartValues", func(t *testing.T) {
		useGpu := true
		dataDir := "some/dir/path"
		expected := Record{
			"tensorleap-engine": Record{
				"gpu":                useGpu,
				"localDataDirectory": dataDir,
			},
			"tensorleap-node-server": Record{
				"disableDatadogMetrics": false,
			},
		}

		result := CreateTensorleapChartValues(useGpu, dataDir, false)

		assert.Equal(t, expected, result)
	})
}

func TestCreateTensorleapChartValuesFormOldValues(t *testing.T) {
	t.Run("With no values", func(t *testing.T) {
		oldValues := Record{
			"tensorleap-engine": Record{
				"localDataDirectory": "some/dir/path",
			},
			"tensorleap-node-server": Record{},
		}
		expected := Record{
			"tensorleap-engine": Record{
				"gpu":                false,
				"localDataDirectory": "some/dir/path",
			},
			"tensorleap-node-server": Record{
				"disableDatadogMetrics": false,
			},
		}

		result, err := CreateTensorleapChartValuesFormOldValues(oldValues)

		assert.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("With values", func(t *testing.T) {

		t.Skip("skipping test") // for debugging

		helmConfig, _ := CreateHelmConfig("", "k3d-tensorleap", "tensorleap")
		oldVals, err := GetValues(helmConfig, "tensorleap")
		if err != nil {
			t.Fatal(err)
		}
		newVals, err := CreateTensorleapChartValuesFormOldValues(oldVals)
		if err != nil {
			t.Fatal(err)
		}
		print(newVals)
	})
}
