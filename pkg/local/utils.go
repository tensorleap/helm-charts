package local

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"time"

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
	MONGODB_STORAGE_DIR_NAME        = "storage/mongodb"
	MINIO_STORAGE_DIR_NAME          = "storage/minio"
	HOSTNAME_FILE                   = "hostname"
	MANIFEST_DIR_NAME               = "manifests"
	INSTALLATION_PARAMS_FILE_NAME   = "params.yaml"
	INSTALLATION_MANIFEST_FILE_NAME = "manifest.yaml"
	KUBECONFIG_FILE_NAME            = "kubeconfig.yaml"
	CONTAINERD_DIR_NAME             = "containerd"
	HELM_CACHE_DIR_NAME             = "helm-cache"
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

// sharedGroupSetup is EnsureSharedGroup; tests replace it so the directory
// policy can be exercised without touching the host's groups.
var sharedGroupSetup = EnsureSharedGroup

// Directory policy for the data dir tree (see the note in files.go):
//   - shared:  humans write here through the CLI; group-owned 2775
//   - storage: pods write here with their own uids; world-writable
//   - cache:   only root inside the k3s node writes here; plain 0755, never
//     copied on a data-dir transfer, rebuilt from the local registry
var (
	sharedDataSubDirs  = []string{MANIFEST_DIR_NAME, LOGS_DIR_NAME, HELM_CACHE_DIR_NAME}
	storageDataSubDirs = []string{STORAGE_DIR_NAME, ELASTIC_STORAGE_DIR_NAME, KEYCLOAK_DB_STORAGE_DIR_NAME, REGISTRY_DIR_NAME}
	cacheDataSubDirs   = []string{CONTAINERD_DIR_NAME}
)

// InitStandaloneDir makes sure the shared group exists with the current user
// active in it, then that the data dir and the subdirs we manage exist with
// the permissions their policy calls for. It runs on every command, so a
// second local user can maintain an install another user created (ubuntu
// installs, ssm-user upgrades) and drift is healed each time.
func InitStandaloneDir() error {
	if err := sharedGroupSetup(); err != nil {
		log.Warnf("Shared group setup incomplete, other local users may be unable to operate this install: %v", err)
	}

	standaloneDir := GetServerDataDir()
	if _, err := os.Stat(standaloneDir); os.IsNotExist(err) {
		log.Printf("Creating directory: %s (you may be asked to enter the root user password)", standaloneDir)
	}
	if err := EnsureSharedDir(standaloneDir); err != nil {
		return err
	}

	return initStandaloneSubDirs()
}

func initStandaloneSubDirs() error {
	standaloneDir := GetServerDataDir()
	for _, dir := range sharedDataSubDirs {
		if err := EnsureSharedDir(path.Join(standaloneDir, dir)); err != nil {
			return err
		}
	}
	for _, dir := range storageDataSubDirs {
		if err := EnsureDirExists(path.Join(standaloneDir, dir)); err != nil {
			return err
		}
	}
	for _, dir := range cacheDataSubDirs {
		if err := EnsurePrivateDir(path.Join(standaloneDir, dir)); err != nil {
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

	SetupK3dLogger(log.VerboseLogger)
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
	for _, dir := range []string{STORAGE_DIR_NAME, REGISTRY_DIR_NAME, CONTAINERD_DIR_NAME, MANIFEST_DIR_NAME, HELM_CACHE_DIR_NAME} {
		path := path.Join(GetServerDataDir(), dir)
		log.Infof("Removing directory: %s", path)
		if err := RemovePath(path); err != nil {
			log.SendCloudReport("error", "Failed purge data", "Failed", &map[string]interface{}{"error": err.Error()})
			return err
		}
	}
	return nil
}

// CleanupCacheData removes the caches (helm charts, container images, registry
// data); all of them are rebuilt by the next install.
func CleanupCacheData() error {
	log.Infof("Cleaning up cache data")
	for _, dir := range []string{HELM_CACHE_DIR_NAME, CONTAINERD_DIR_NAME, REGISTRY_DIR_NAME} {
		if err := RemovePath(path.Join(GetServerDataDir(), dir)); err != nil {
			return err
		}
	}
	return nil
}

// ClearAppData removes application data (storage) but keeps cache (registry, containerd, helm-cache)
func ClearAppData() error {
	log.Infof("Clearing application data (you may be asked to enter the root user password)")
	// Remove storage directory (contains keycloak db, elasticsearch data, etc.)
	storagePath := path.Join(GetServerDataDir(), STORAGE_DIR_NAME)
	log.Infof("Removing directory: %s", storagePath)
	if err := RemovePath(storagePath); err != nil {
		log.SendCloudReport("error", "Failed to clear app data", "Failed", &map[string]interface{}{"error": err.Error()})
		return err
	}

	// Remove manifests directory (installation config)
	manifestsPath := path.Join(GetServerDataDir(), MANIFEST_DIR_NAME)
	log.Infof("Removing directory: %s", manifestsPath)
	if err := RemovePath(manifestsPath); err != nil {
		log.SendCloudReport("error", "Failed to clear manifests", "Failed", &map[string]interface{}{"error": err.Error()})
		return err
	}

	return nil
}

// RemoveDataSubDir removes a single path (file or directory) under the server
// data dir, falling back to sudo when a permission error blocks direct removal.
// Used by the custom uninstall to delete only the items the user selected.
func RemoveDataSubDir(subDir string) error {
	target := path.Join(GetServerDataDir(), subDir)
	log.Infof("Removing: %s", target)
	if err := RemovePath(target); err != nil {
		log.SendCloudReport("error", "Failed removing data path", "Failed", &map[string]interface{}{"path": target, "error": err.Error()})
		return err
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

// GetKubeConfigPath is the shared kubeconfig any local user's kubectl/helm can
// point at via $KUBECONFIG. Lives in the manifest dir alongside the other
// install artifacts.
func GetKubeConfigPath() string {
	return path.Join(GetServerDataDir(), MANIFEST_DIR_NAME, KUBECONFIG_FILE_NAME)
}

func GetContainerdDataDir() string {
	return path.Join(GetServerDataDir(), CONTAINERD_DIR_NAME)
}

func GetHelmCacheDir() string {
	return path.Join(GetServerDataDir(), HELM_CACHE_DIR_NAME)
}
