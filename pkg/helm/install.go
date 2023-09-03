package helm

import (
	"time"

	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
)

func InstallChart(
	config *HelmConfig,
	chartMeta manifest.HelmChartMeta,
	chart *chart.Chart,
	values Record,
) error {

	log.Println("Installing helm chart (will take few minutes)")

	client := action.NewInstall(config.ActionConfig)
	client.Namespace = config.Namespace
	client.CreateNamespace = true
	client.Wait = true
	client.ReleaseName = chartMeta.ReleaseName
	client.Timeout = 20 * time.Minute

	_, err := client.RunWithContext(config.Context, chart, values)
	if err != nil {
		return err
	}

	log.Printf("Tensorleap installed on local k3d cluster! version: %s", chart.Metadata.Version)

	return nil
}
