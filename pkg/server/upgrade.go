package server

import (
	"context"
	"fmt"
	"os"

	"github.com/tensorleap/helm-charts/pkg/k3d"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
)

func ValidateStandaloneDir() error {
	standaloneDir := local.GetServerDataDir()
	_, err := os.Stat(standaloneDir)
	if os.IsNotExist(err) {
		log.SendCloudReport("error", "Installation dir not found", "Failed", &map[string]interface{}{"error": err.Error()})
		return fmt.Errorf("not found data directory(%s) on this machine, Please make sure to install before upgrade", standaloneDir)
	}
	return err
}

func ValidateClusterExists(ctx context.Context) error {
	cluster, err := k3d.GetCluster(ctx)
	if err != nil {
		log.SendCloudReport("error", "Failed getting k3d cluster", "Failed", &map[string]interface{}{"error": err.Error()})
		return err
	}
	if cluster == nil {
		log.SendCloudReport("error", "K3d cluster found was null", "Failed", &map[string]interface{}{"error": err.Error()})
		return fmt.Errorf("not found local Cluster(%s) on this machine, Please make sure to install before upgrade", local.GetServerDataDir())
	}
	return nil
}
