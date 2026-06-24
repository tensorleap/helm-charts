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
	return installChart(config, releaseName, chart, values, true, 4*time.Hour)
}

// InstallChartNoWait submits the chart manifests and returns immediately without
// waiting for pods to become ready. Use when readiness is monitored separately.
func InstallChartNoWait(
	config *HelmConfig,
	releaseName string,
	chart *chart.Chart,
	values Record,
) error {
	return installChart(config, releaseName, chart, values, false, 5*time.Minute)
}

func installChart(
	config *HelmConfig,
	releaseName string,
	chart *chart.Chart,
	values Record,
	wait bool,
	timeout time.Duration,
) error {
	log.Printf("Installing helm chart %s (will take few minutes)", releaseName)

	client := action.NewInstall(config.ActionConfig)
	client.Namespace = config.Namespace
	client.CreateNamespace = true
	client.Wait = wait
	client.ReleaseName = releaseName
	client.Timeout = timeout

	_, err := client.RunWithContext(config.Context, chart, values)
	if err != nil {
		return err
	}

	log.Printf("Installed helm chart: %s version: %s", releaseName, chart.Metadata.Version)
	return nil
}
