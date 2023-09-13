package helm

import (
	"time"

	"github.com/tensorleap/helm-charts/pkg/log"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
)

func UpgradeChart(
	config *HelmConfig,
	releaseName string,
	chart *chart.Chart,
	values Record,
) error {

	log.SendCloudReport("info", "Upgrading helm chart", "Running", nil)
	log.Println("Upgrading helm chart (will take few minutes)")

	client := action.NewUpgrade(config.ActionConfig)
	client.Namespace = config.Namespace
	client.Wait = true
	client.Timeout = 20 * time.Minute

	_, err := client.RunWithContext(config.Context, releaseName, chart, values)
	if err != nil {
		log.SendCloudReport("error", "Failed upgrading helm chart", "Failed",
			&map[string]interface{}{"releaseName": releaseName, "latestChart": chart, "error": err.Error()})
		return err
	}

	log.Printf("Tensorleap upgrade on local k3d cluster! version: %s", chart.Metadata.Version)
	log.SendCloudReport("info", "Successfully upgraded helm chart", "Running", nil)

	return nil
}
