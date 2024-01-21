package helm

import (
	"time"

	"github.com/tensorleap/helm-charts/pkg/log"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
)

func InstallChart(
	config *HelmConfig,
	releaseName string,
	chart *chart.Chart,
	values Record,
) error {

	log.Printf("Installing helm chart %s (will take few minutes)", releaseName)

	client := action.NewInstall(config.ActionConfig)
	client.Namespace = config.Namespace
	client.CreateNamespace = true
	client.Wait = true
	client.ReleaseName = releaseName
	client.Timeout = 30 * time.Minute

	_, err := client.RunWithContext(config.Context, chart, values)
	if err != nil {
		return err
	}

	log.Printf("Installed helm chart: %s version: %s", releaseName, chart.Metadata.Version)
	return nil
}
