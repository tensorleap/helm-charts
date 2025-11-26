package helm

import (
	"bufio"
	"context"
	"embed"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/storage/driver"
)

type Record = map[string]interface{}

type TLSParams struct {
	Enabled bool   `json:"enabled"`
	Cert    string `json:"cert"`
	Key     string `json:"key"`
}

type ServerHelmValuesParams struct {
	Gpu                   bool              `json:"gpu"`
	LocalDataDirectories  []string          `json:"localDataDirectories"`
	DisableDatadogMetrics bool              `json:"disableDatadogMetrics"`
	Domain                string            `json:"domain"`
	BasePath              string            `json:"basePath"`
	Url                   string            `json:"url"`
	ProxyUrl              string            `json:"proxyUrl"`
	Tls                   TLSParams         `json:"tls"`
	HostName              string            `json:"hostname"`
	DatadogEnv            map[string]string `json:"datadogEnv"`
	KeycloakEnabled       bool              `json:"keycloakEnabled"`
}

type InfraHelmValuesParams struct {
	NvidiaGpuEnable         bool   `json:"nvidiaGpuEnable"`
	NvidiaGpuVisibleDevices string `json:"nvidiaGpuVisibleDevices"`
}

var ErrNoRelease = fmt.Errorf("no release")

const HOSTNAME_SUFFIX string = ".on-prem"

func IsHelmReleaseExists(config *HelmConfig, releaseName string) (bool, error) {
	_, err := GetHelmReleaseVersion(config, releaseName)
	if err == ErrNoRelease {
		return false, nil
	}
	return err == nil, err
}

func GetHelmReleaseVersion(config *HelmConfig, releaseName string) (string, error) {
	client := action.NewHistory(config.ActionConfig)
	client.Max = 0 // 0 means fetch all history
	history, err := client.Run(releaseName)

	if err == driver.ErrReleaseNotFound {
		return "", ErrNoRelease
	} else if err != nil || len(history) == 0 {
		log.SendCloudReport("error", "Failed getting helm release version", "Failed", &map[string]interface{}{"error": err.Error(), releaseName: releaseName})
		return "", fmt.Errorf("failed getting helm release version: %s", err.Error())
	}

	// Find the latest deployed/successful release
	for i := len(history) - 1; i >= 0; i-- {
		rel := history[i]
		if rel.Info != nil && rel.Info.Status == "deployed" {
			return rel.Chart.Metadata.Version, nil
		}
	}
	// If no deployed release found, fallback to latest entry
	return history[len(history)-1].Chart.Metadata.Version, nil
}

//go:embed resources/*
var dictFiles embed.FS

func readFileToList(fs embed.FS, filePath string) ([]string, error) {
	fileData, err := fs.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(fileData)))
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func loadWordList(listName string) ([]string, error) {
	fileName := fmt.Sprintf("resources/%s_en.txt", listName)
	list, err := readFileToList(dictFiles, fileName)
	if err != nil {
		fmt.Println("Error reading file:", err)
		return nil, err
	}
	return list, nil
}

func loadAdjectiveList() ([]string, error) {
	return loadWordList("adjectives")
}

func loadAnimalList() ([]string, error) {
	return loadWordList("animals")
}

func generateRandomName(seed *int64) (string, error) {
	var s int64
	if seed != nil {
		s = *seed
	} else {
		s = time.Now().UnixNano()
	}
	var r = rand.New(rand.NewSource(s))
	adjectives, err := loadAdjectiveList()
	if err != nil {
		fmt.Println("Error generating random name:", err)
		return "", err
	}
	adjective := adjectives[r.Intn(len(adjectives))]

	animals, err := loadAnimalList()
	if err != nil {
		fmt.Println("Error generating random name:", err)
		return "", err
	}
	animal := animals[r.Intn(len(animals))]
	return fmt.Sprintf("%s-%s", adjective, animal), nil
}

func persistHostname(hostname string) error {
	filePath := local.GetInstallationHostnamePath()
	err := os.WriteFile(filePath, []byte(hostname), 0644)

	if err != nil {
		return fmt.Errorf("error persisting hostname: %v", err)
	}
	return nil
}

// Read hostname from /var/lib/tensorleap or return empty if file does not exist
func readHostname() (string, error) {
	filePath := local.GetInstallationHostnamePath()
	data, err := os.ReadFile(filePath) // Reading the content of the file

	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // Returning an empty string and nil error if the file does not exist
		}
		return "", fmt.Errorf("error reading hostname: %v", err) // Returning the error if other error occurs
	}

	return string(data), nil
}

func readOrGenerateHostname() (string, error) {
	existingHostName, err := readHostname()
	if err != nil || existingHostName == "" {
		freshName, err := generateRandomName(nil)
		if err != nil {
			fmt.Println("Unable to generate hostname", err)
			return "", err
		}
		hostname := freshName + HOSTNAME_SUFFIX
		err = persistHostname(hostname)
		if err != nil {
			fmt.Println("Unable to persist hostname to local data dir", err)
			return "", err
		}
		return hostname, nil
	} else {
		return existingHostName, nil
	}
}

func CreateTensorleapChartValues(params *ServerHelmValuesParams) (Record, error) {
	var hostname string
	var err error
	if params.HostName != "" {
		hostname = params.HostName
	} else {
		hostname, err = readOrGenerateHostname()
		if err != nil {
			fmt.Println("Error generating hostname", err)
			return nil, err
		}
	}

	type ExtraEnv struct {
		Name  string `yaml:"name"`
		Value string `yaml:"value"`
	}

	extraEnvSlice := []ExtraEnv{
		{Name: "KEYCLOAK_ADMIN", Value: "admin"},
		{Name: "KEYCLOAK_ADMIN_PASSWORD", Value: "admin"},
		{Name: "KC_DB", Value: "dev-file"},
		{Name: "KC_CACHE", Value: "local"},
		{Name: "KC_HTTP_RELATIVE_PATH", Value: "/auth"},
		{Name: "KC_PROXY", Value: "edge"},
		{Name: "KC_PROXY_HEADERS", Value: "forwarded"},
		{Name: "KC_CACHE_STACK", Value: ""},
	}

	extraEnvSlice = append(extraEnvSlice, ExtraEnv{Name: "KC_HTTP_ENABLED", Value: "true"})

	if params.Domain != "" && params.Domain != "localhost" {
		extraEnvSlice = append(extraEnvSlice, ExtraEnv{Name: "KC_HOSTNAME", Value: params.Domain})
		extraEnvSlice = append(extraEnvSlice, ExtraEnv{Name: "KC_HOSTNAME_STRICT", Value: "true"})
		extraEnvSlice = append(extraEnvSlice, ExtraEnv{Name: "KC_PROXY_HEADERS", Value: "xforwarded"})

		if params.ProxyUrl != "" {
			extraEnvSlice = append(extraEnvSlice, ExtraEnv{Name: "KC_HOSTNAME_URL", Value: fmt.Sprintf("%s/auth", params.ProxyUrl)})
			extraEnvSlice = append(extraEnvSlice, ExtraEnv{Name: "KC_FRONTEND_URL", Value: fmt.Sprintf("%s/auth", params.ProxyUrl)})
		} else if params.Url != "" {
			extraEnvSlice = append(extraEnvSlice, ExtraEnv{Name: "KC_HOSTNAME_URL", Value: fmt.Sprintf("%s/auth", params.Url)})
			extraEnvSlice = append(extraEnvSlice, ExtraEnv{Name: "KC_FRONTEND_URL", Value: fmt.Sprintf("%s/auth", params.Url)})
		}
	} else {
		extraEnvSlice = append(extraEnvSlice, ExtraEnv{Name: "KC_HOSTNAME_STRICT", Value: "false"})
		extraEnvSlice = append(extraEnvSlice, ExtraEnv{Name: "KC_PROXY_HEADERS", Value: "forwarded"})
	}

	extraEnvSlice = append(extraEnvSlice, ExtraEnv{Name: "KC_HOSTNAME_STRICT_HTTPS", Value: strconv.FormatBool(params.Tls.Enabled)})

	if params.Tls.Enabled && params.Url != "" {
		extraEnvSlice = append(extraEnvSlice, ExtraEnv{Name: "KC_HOSTNAME_ADMIN_URL", Value: fmt.Sprintf("%s/auth", params.Url)})
	}
	formatExtraEnv := func(extraEnv []ExtraEnv) string {
		result, _ := yaml.Marshal(extraEnv)
		resultString := "\n" + string(result)
		return resultString
	}
	extraEnvStringYaml := formatExtraEnv(extraEnvSlice)

	datadogEnvs := []map[string]string{}
	datadogEnvs = append(datadogEnvs, map[string]string{"name": "DD_HOSTNAME", "value": hostname})

	for key, value := range params.DatadogEnv {
		datadogEnvs = append(datadogEnvs, map[string]string{"name": key, "value": value})
	}

	return Record{
		"tensorleap-engine": Record{
			"gpu":                  params.Gpu,
			"localDataDirectories": params.LocalDataDirectories,
		},
		"tensorleap-node-server": Record{
			"enableKeycloak":        params.KeycloakEnabled,
			"disableDatadogMetrics": params.DisableDatadogMetrics,
		},
		"global": Record{
			"domain":               params.Domain,
			"url":                  params.Url,
			"proxyUrl":             params.ProxyUrl,
			"basePath":             params.BasePath,
			"create_local_volumes": true,
			"storageClassName":     "",
			"keycloakx": Record{
				"enabled": params.KeycloakEnabled,
			},
			"tls": Record{
				"enabled": params.Tls.Enabled,
				"cert":    params.Tls.Cert,
				"key":     params.Tls.Key,
			},
		},
		"keycloakx": map[string]interface{}{
			"enabled":  params.KeycloakEnabled,
			"replicas": 1,
			"command":  []interface{}{"/opt/keycloak/bin/kc.sh", "start"},
			"extraEnv": extraEnvStringYaml,
		},
		"datadog": map[string]interface{}{
			"enabled": !params.DisableDatadogMetrics,
			"datadog": map[string]interface{}{
				"env": datadogEnvs,
			},
		},
	}, nil
}

func CreateInfraChartValues(params *InfraHelmValuesParams) Record {
	return Record{
		"nvidiaGpu": Record{
			"enabled":        params.NvidiaGpuEnable,
			"visibleDevices": params.NvidiaGpuVisibleDevices,
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
