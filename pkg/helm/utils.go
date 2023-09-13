package helm

import (
	"context"
	"fmt"
	"os"

	"github.com/tensorleap/helm-charts/pkg/log"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/storage/driver"
)

type Record = map[string]interface{}

type ServerHelmValuesParams struct {
	Gpu                   bool   `json:"gpu"`
	LocalDataDirectory    string `json:"localDataDirectory"`
	DisableDatadogMetrics bool   `json:"disableDatadogMetrics"`
}

var ErrNoRelease = fmt.Errorf("no release")

func IsHelmReleaseExists(config *HelmConfig, releaseName string) (bool, error) {
	_, err := GetHelmReleaseVersion(config, releaseName)
	if err == ErrNoRelease {
		return false, nil
	}
	return err == nil, err
}

func GetHelmReleaseVersion(config *HelmConfig, releaseName string) (string, error) {
	client := action.NewHistory(config.ActionConfig)
	client.Max = 1
	history, err := client.Run(releaseName)

	if err == driver.ErrReleaseNotFound {
		return "", ErrNoRelease
	} else if err != nil || len(history) == 0 {
		log.SendCloudReport("error", "Failed getting helm release version", "Failed", &map[string]interface{}{"error": err.Error(), releaseName: releaseName})
		return "", fmt.Errorf("failed getting helm release version: %s", err.Error())
	}

	return history[0].Chart.Metadata.Version, nil
}

func CreateTensorleapChartValues(params *ServerHelmValuesParams) Record {
	return Record{
		"tensorleap-engine": Record{
			"gpu":                params.Gpu,
			"localDataDirectory": params.LocalDataDirectory,
		},
		"tensorleap-node-server": Record{
			"disableDatadogMetrics": params.DisableDatadogMetrics,
		},
	}
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
