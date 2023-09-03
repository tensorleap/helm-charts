package helm

import (
	"context"
	"fmt"
	"os"

	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/storage/driver"
)

type Record = map[string]interface{}

func IsHelmReleaseExists(config *HelmConfig, helmChart manifest.HelmChartMeta) (bool, error) {
	client := action.NewHistory(config.ActionConfig)
	client.Max = 1
	_, err := client.Run(helmChart.ReleaseName)
	if err == driver.ErrReleaseNotFound {
		return false, nil
	} else if err != nil {
		log.SendCloudReport("error", "Failed validating helm release exists", "Failed", &map[string]interface{}{"error": err.Error()})
		return false, err
	}
	return true, nil
}

func CreateTensorleapChartValues(useGpu bool, dataDir string, disableMetrics bool) Record {
	return Record{
		"tensorleap-engine": Record{
			"gpu":                useGpu,
			"localDataDirectory": dataDir,
		},
		"tensorleap-node-server": Record{
			"disableDatadogMetrics": disableMetrics,
		},
	}
}

func CreateTensorleapChartValuesFormOldValues(oldValues Record) (Record, error) {

	disableMetrics := false
	useGpu := false

	errFailedGettingOldValue := fmt.Errorf("failed getting old values")

	engineVal, ok := oldValues["tensorleap-engine"]
	if !ok {
		return nil, errFailedGettingOldValue
	}
	engineValMap, ok := engineVal.(map[string]interface{})
	if !ok {
		return nil, errFailedGettingOldValue
	}
	useGpuVal, ok := engineValMap["gpu"]
	if ok {
		useGpu, _ = useGpuVal.(bool)
	}
	dataDirVal, ok := engineValMap["localDataDirectory"]
	if !ok {
		return nil, errFailedGettingOldValue
	}
	dataDir, ok := dataDirVal.(string)
	if !ok {
		return nil, fmt.Errorf("failed getting old data directory value, try to install instead of upgrade")
	}

	nodeServerVal, ok := oldValues["tensorleap-node-server"]
	if ok {
		nodeServerMap, ok := nodeServerVal.(map[string]interface{})
		if ok {
			disableMetrics, _ = nodeServerMap["disableDatadogMetrics"].(bool)
		}
	}

	newValues := CreateTensorleapChartValues(useGpu, dataDir, disableMetrics)
	return newValues, nil
}

func GetValues(config *HelmConfig, releaseName string) (Record, error) {
	client := action.NewGetValues(config.ActionConfig)
	client.AllValues = true
	return client.Run(releaseName)
}

type HelmConfig struct {
	Namespace    string
	Context      context.Context
	ActionConfig *action.Configuration
	Settings     *cli.EnvSettings
}

func CreateHelmConfig(kubeConfigPath string, kubeContext, namespace string) (*HelmConfig, error) {
	settings := cli.New()
	settings.SetNamespace(namespace)
	settings.KubeContext = kubeContext

	if len(kubeConfigPath) > 0 {
		settings.KubeConfig = kubeConfigPath
	}

	// Any other context with cancel will failed immediately when running helm actions, using background context solve it
	ctx := context.Background()
	helmDriver := os.Getenv("HELM_DRIVER")

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), os.Getenv("HELM_DRIVER"), log.VerboseLogger.Printf); err != nil {
		log.SendCloudReport("error", "Failed creating helm config", "Failed",
			&map[string]interface{}{"namespace": namespace, "helmDriver": helmDriver, "error": err.Error()})
		return nil, err
	}

	log.SendCloudReport("info", "Successfully created helm config", "Running", nil)
	return &HelmConfig{
		Context:      ctx,
		Namespace:    namespace,
		ActionConfig: actionConfig,
		Settings:     settings,
	}, nil
}
