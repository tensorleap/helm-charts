package helm

import (
	"fmt"
	"time"

	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
)

func UpgradeTensorleapChartVersion(
	config *HelmConfig,
	chartMeta manifest.HelmChartMeta,
	chart *chart.Chart,
	values Record,
) error {

	log.SendCloudReport("info", "Upgrading helm chart", "Running", nil)
	log.Println("Upgrading helm chart (will take few minutes)")

	client := action.NewUpgrade(config.ActionConfig)
	client.Namespace = config.Namespace
	client.Wait = true
	if values == nil {
		oldValues, err := GetValues(config, chartMeta.ReleaseName)
		if err != nil {
			return err
		}
		values, err = CreateTensorleapChartValuesFormOldValues(oldValues)
		if err != nil {
			return fmt.Errorf("failed creating values from previous helm release: %w", err)
		}
	}

	client.Timeout = 20 * time.Minute

	_, err := client.RunWithContext(config.Context, chartMeta.ReleaseName, chart, values)
	if err != nil {
		log.SendCloudReport("error", "Failed upgrading helm chart", "Failed",
			&map[string]interface{}{"releaseName": chartMeta.ReleaseName, "latestChart": chart, "error": err.Error()})
		return err
	}

	log.Printf("Tensorleap upgrade on local k3d cluster! version: %s", chart.Metadata.Version)
	log.SendCloudReport("info", "Successfully upgraded helm chart", "Running", nil)

	return nil
}
