package chart

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/tensorleap/helm-charts/pkg/log"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/repo"
)

type Chart = chart.Chart

func IsLocalChart(repo string) bool {
	return !strings.HasPrefix(repo, "http")
}

func Load(repo, chartName, version string) (chart *Chart, err error) {
	isLocal := IsLocalChart(repo)
	if isLocal {
		chart, err = loader.Load(filepath.Join(repo, chartName))
		if err != nil {
			return nil, err
		}
		return chart, nil
	}
	chartFile, _, err := DownloadIntoTempFile(repo, chartName, version)
	if err != nil {
		return nil, err
	}
	tmpChartPath := chartFile.Name()
	chartFile.Close()
	defer os.Remove(tmpChartPath)
	chart, err = loader.Load(chartFile.Name())
	if err != nil {
		log.SendCloudReport("error", "Failed loading helm chart", "Failed", &map[string]interface{}{"error": err.Error()})
		return nil, err
	}
	return
}

func Save(chart *Chart, outDir string) (absPath string, err error) {
	return chartutil.Save(chart, outDir)
}

func DownloadIntoTempFile(repo, chartName, version string) (*os.File, func(), error) {
	log.Info("Downloading helm chart...")
	chartVersion, err := GetVersion(repo, chartName, version)
	if err != nil {
		return nil, nil, err
	}
	return downloadIntoTempFile(chartVersion.URLs[0], "helm.tgz")
}

func downloadIntoTempFile(url, tempSuffix string) (*os.File, func(), error) {
	tempFile, err := os.CreateTemp("", tempSuffix)
	if err != nil {
		return nil, nil, err
	}
	clean := func() {
		tempFile.Close()
		os.Remove(tempFile.Name())
	}

	res, err := http.Get(url)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("failed downloading (%s): %v", url, res.StatusCode)
	}
	_, err = io.Copy(tempFile, res.Body)
	if err != nil {
		return nil, nil, err
	}
	_, err = tempFile.Seek(0, 0)
	if err != nil {
		return nil, nil, err
	}

	return tempFile, clean, nil
}

// GetVersion returns version of a helm chart if version is empty it returns the latest version
func GetVersion(repoUrl, chartName, version string) (*repo.ChartVersion, error) {

	url, err := repo.ResolveReferenceURL(repoUrl, "index.yaml")
	if err != nil {
		return nil, err
	}

	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	tempFile, err := os.CreateTemp("", "index.yaml")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tempFile.Name())
	_, err = io.Copy(tempFile, res.Body)
	if err != nil {
		return nil, err
	}
	path := tempFile.Name()
	res.Body.Close()
	tempFile.Close()

	indexFile, err := repo.LoadIndexFile(path)
	if err != nil {
		return nil, err
	}
	return indexFile.Get(chartName, version)
}
