package local

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"time"

	"github.com/tensorleap/helm-charts/pkg/k3d"
	"github.com/tensorleap/helm-charts/pkg/k8s"
	"github.com/tensorleap/helm-charts/pkg/log"
)

const (
	DATA_DIR_ENV_NAME               = "TL_DATA_DIR"
	DEFAULT_DATA_DIR                = "/var/lib/tensorleap/standalone"
	REGISTRY_DIR_NAME               = "registry"
	LOGS_DIR_NAME                   = "logs"
	STORAGE_DIR_NAME                = "storage"
	KEYCLOAK_DB_STORAGE_DIR_NAME    = "storage/keycloak"
	ELASTIC_STORAGE_DIR_NAME        = "storage/elasticsearch"
	HOSTNAME_FILE                   = "hostname"
	MANIFEST_DIR_NAME               = "manifests"
	INSTALLATION_PARAMS_FILE_NAME   = "params.yaml"
	INSTALLATION_MANIFEST_FILE_NAME = "manifest.yaml"
)

func GetServerDataDir() string {
	envValue := os.Getenv(DATA_DIR_ENV_NAME)
	if envValue != "" {
		return envValue
	}
	return DEFAULT_DATA_DIR
}

var previousDataDir string

func GetPreviousServerDataDir() string {
	return previousDataDir
}

func SetDataDir(previous, flag string) error {
	previousDataDir = previous
	currentDataDir := flag
	if currentDataDir == "" {
		currentDataDir = os.Getenv(DATA_DIR_ENV_NAME)
	}
	if currentDataDir == "" {
		currentDataDir = previous
	}
	currentDataDir, err := filepath.Abs(currentDataDir)
	if err != nil {
		return err
	}

	os.Setenv(DATA_DIR_ENV_NAME, currentDataDir)
	return nil
}

func InitStandaloneDir() error {
	standaloneDir := GetServerDataDir()
	_, err := os.Stat(standaloneDir)
	if os.IsNotExist(err) {
		log.Printf("Creating directory: %s (you may be asked to enter the root user password)", standaloneDir)
		mkdirCmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("sudo mkdir -p %s", standaloneDir))
		if err := mkdirCmd.Run(); err != nil {
			return err
		}

		log.Println("Setting directory permissions")
		chmodCmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("sudo chmod -R 777 %s", standaloneDir))
		if err := chmodCmd.Run(); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		log.Printf("Directory %s already exists, check permission", standaloneDir)
		info, err := os.Stat(standaloneDir)
		if err != nil || info.Mode().Perm() != 0777 {
			log.Printf("Setting directory permissions (you may be asked to enter the root user password)")
			chmodCmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("sudo chmod -R 777 %s", standaloneDir))
			if err := chmodCmd.Run(); err != nil {
				return err
			}
		}
	}

	return initStandaloneSubDirs()
}

func initStandaloneSubDirs() error {
	standaloneDir := GetServerDataDir()
	subDirs := []string{STORAGE_DIR_NAME, REGISTRY_DIR_NAME, LOGS_DIR_NAME, MANIFEST_DIR_NAME, KEYCLOAK_DB_STORAGE_DIR_NAME, ELASTIC_STORAGE_DIR_NAME}
	for _, dir := range subDirs {
		fullPath := path.Join(standaloneDir, dir)
		_, err := os.Stat(fullPath)
		if os.IsNotExist(err) {
			log.Printf("Creating directory: %s", fullPath)
			if err := os.MkdirAll(fullPath, 0777); err != nil {
				return err
			}
			// the permission of the directory not set to 0777 even if we set it in the MkdirAll
			if err := os.Chmod(fullPath, 0777); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	return nil
}

// SetupInfra init VAR_DIR, setup VerboseLog and connect its output into a file
func SetupInfra(cmdName string) (closeLogFile func(), err error) {
	err = InitStandaloneDir()
	if err != nil {
		log.SendCloudReport("error", "Failed initializing standalone dir", "Failed", &map[string]interface{}{"error": err.Error()})
		return
	}

	k3d.SetupLogger(log.VerboseLogger)
	k8s.SetupLogger(log.VerboseLogger)

	logPath := createLogFilePath(cmdName)
	closeLogFile, err = log.ConnectFileToVerboseLogOutput(logPath)

	log.SendCloudReport("info", "Finished setting cli infra", "Running", nil)
	return
}

func createLogFilePath(cmdName string) string {
	filePath := fmt.Sprintf("%s/logs/%s_%s.log",
		GetServerDataDir(),
		cmdName,
		time.Now().Format("2006-01-02_15-04-05"),
	)
	return filePath
}

func OpenLink(link string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", link)
	case "linux":
		cmd = exec.Command("xdg-open", link)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", link)
	default:
		return fmt.Errorf("unsupported platform")
	}

	return cmd.Start()
}

func PurgeData() error {
	log.Infof("Purging data (you may be asked to enter the root user password)")
	for _, dir := range []string{STORAGE_DIR_NAME, REGISTRY_DIR_NAME, MANIFEST_DIR_NAME} {
		path := path.Join(GetServerDataDir(), dir)
		log.Infof("Removing directory: %s", path)
		err := os.RemoveAll(path)

		// if failed to remove directory, try to remove it with sudo
		if err != nil {
			rmCmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("sudo rm -rf %s", path))

			if err := rmCmd.Run(); err != nil {
				log.SendCloudReport("error", "Failed purge data", "Failed", &map[string]interface{}{"error": err.Error()})
				return err
			}
		}
	}
	return nil
}

func GetInstallationManifestPath() string {
	return path.Join(GetServerDataDir(), MANIFEST_DIR_NAME, INSTALLATION_MANIFEST_FILE_NAME)
}

func GetInstallationHostnamePath() string {
	return path.Join(GetServerDataDir(), HOSTNAME_FILE)
}

func GetInstallationParamsPath() string {
	return path.Join(GetServerDataDir(), MANIFEST_DIR_NAME, INSTALLATION_PARAMS_FILE_NAME)
}
