package k3d

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	cliutil "github.com/k3d-io/k3d/v5/cmd/util"
	k3dCluster "github.com/k3d-io/k3d/v5/pkg/client"
	"github.com/k3d-io/k3d/v5/pkg/config"
	k3dConfTypes "github.com/k3d-io/k3d/v5/pkg/config/types"
	conf "github.com/k3d-io/k3d/v5/pkg/config/v1alpha5"
	"github.com/k3d-io/k3d/v5/pkg/runtimes"
	k3d "github.com/k3d-io/k3d/v5/pkg/types"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
)

type Cluster = k3d.Cluster

const CLUSTER_NAME = "tensorleap"

func GetCluster(ctx context.Context) (*Cluster, error) {
	clusters, err := k3dCluster.ClusterList(ctx, runtimes.SelectedRuntime)
	if err != nil {
		return nil, err
	}

	for _, cluster := range clusters {
		if cluster.Name == CLUSTER_NAME {
			return cluster, nil
		}
	}

	return nil, nil
}

func CreateCluster(ctx context.Context, manifest *manifest.InstallationManifest, port uint, volumes []string, useGpu bool) error {
	log.SendCloudReport("info", "Creating cluster", "Running", &map[string]interface{}{"useGpu": useGpu, "port": port})
	clusterConfig := createClusterConfig(ctx, manifest, port, volumes, useGpu)

	if _, err := k3dCluster.ClusterGet(ctx, runtimes.SelectedRuntime, &clusterConfig.Cluster); err == nil {
		log.Println("Found existing tensorleap cluster!")

		log.SendCloudReport("info", "Cluster already exists", "Running", &map[string]interface{}{"useGpu": useGpu, "port": port})
		err = RunCluster(ctx)
		if err != nil {
			return err
		}
		return nil
	}

	if err := k3dCluster.ClusterRun(ctx, runtimes.SelectedRuntime, clusterConfig); err != nil {
		log.Println(err)
		log.Println("Failed to create cluster >>> Rolling Back")
		log.SendCloudReport("error", "Failed creating cluster", "Failed",
			&map[string]interface{}{"selectedRuntime": runtimes.SelectedRuntime, "error": err.Error()})
		if err := k3dCluster.ClusterDelete(ctx, runtimes.SelectedRuntime, &clusterConfig.Cluster, k3d.ClusterDeleteOpts{SkipRegistryCheck: true}); err != nil {
			log.Println(err)
			log.Fatalln("Cluster creation FAILED, also FAILED to rollback changes!")
			log.SendCloudReport("error", "Failed rolling back cluster changes", "Failed",
				&map[string]interface{}{"error": err.Error()})
		}
		log.SendCloudReport("error", "Successfully rolled back cluster changes", "Failed", nil)
		log.Fatalln("Cluster creation FAILED, all changes have been rolled back!")
	}
	log.Printf("Cluster '%s' created successfully!\n", clusterConfig.Cluster.Name)
	log.SendCloudReport("info", "Created cluster successfully", "Running", nil)

	if _, err := k3dCluster.KubeconfigGetWrite(ctx, runtimes.SelectedRuntime, &clusterConfig.Cluster, "", &k3dCluster.WriteKubeConfigOptions{
		UpdateExisting:       true,
		OverwriteExisting:    false,
		UpdateCurrentContext: true,
	}); err != nil {
		log.Println(err)
	}

	return nil
}

func CreateTmpClusterKubeConfig(ctx context.Context, cluster *Cluster) (string, func(), error) {
	kubeConfig, err := k3dCluster.KubeconfigGet(ctx, runtimes.SelectedRuntime, cluster)

	if err != nil {
		log.SendCloudReport("error", "Failed getting cluster kubeconfig", "Failed",
			&map[string]interface{}{"cluster": cluster, "error": err.Error()})
		return "", nil, err
	}
	tmpConfigFile, err := os.CreateTemp("", "kubeconfig")
	if err != nil {
		return "", nil, err
	}
	tmpPath := tmpConfigFile.Name()
	tmpConfigFile.Close()
	cleanup := func() { os.Remove(tmpPath) }

	err = k3dCluster.KubeconfigWriteToPath(ctx, kubeConfig, tmpPath)
	if err != nil {
		log.SendCloudReport("error", "Failed writing cluster kubeconfig", "Failed",
			&map[string]interface{}{"kubeConfig": kubeConfig, "tmpPath": tmpPath, "error": err.Error()})
		cleanup()
		return "", nil, err
	}

	return tmpPath, cleanup, nil
}

// StopCluster stops a cluster if it exists, copy from pkg/k3d/cmd/cluster/stop.go
func StopCluster(ctx context.Context) error {
	cluster, err := GetCluster(ctx)
	if err != nil {
		log.SendCloudReport("error", "Failed getting cluster", "Failed", &map[string]interface{}{"error": err.Error()})
		return err
	}
	if cluster == nil {
		log.SendCloudReport("info", "Cluster not found", "Running", nil)
		log.Info("Cluster 'tensorleap' not found")
		return nil
	}
	log.Info("Stopping cluster 'tensorleap'")
	err = k3dCluster.ClusterStop(ctx, runtimes.SelectedRuntime, cluster)
	if err != nil {
		log.SendCloudReport("error", "Failed stopping cluster", "Failed",
			&map[string]interface{}{"cluster": cluster, "error": err.Error()})
	}
	return err
}

// RunCluster starts a cluster if it exists, copy from pkg/k3d/cmd/cluster/start.go
func RunCluster(ctx context.Context) error {
	cluster, err := GetCluster(ctx)
	if err != nil {
		log.SendCloudReport("error", "Failed getting cluster", "Failed", &map[string]interface{}{"error": err.Error()})
		return err
	}
	if cluster == nil {
		log.SendCloudReport("info", "Cluster not found", "Running", nil)
		log.Info("Cluster 'tensorleap' not found")
		return nil
	}
	log.Info("Running cluster 'tensorleap'")

	startClusterOpts := k3d.ClusterStartOpts{}
	envInfo, err := k3dCluster.GatherEnvironmentInfo(ctx, runtimes.SelectedRuntime, cluster)
	if err != nil {
		return fmt.Errorf("failed to gather info about cluster environment: %v", err)
	}
	startClusterOpts.EnvironmentInfo = envInfo

	// Get pre-defined clusterStartOpts from cluster
	fetchedClusterStartOpts, err := k3dCluster.GetClusterStartOptsFromLabels(cluster)
	if err != nil {
		return fmt.Errorf("failed to get cluster start opts from cluster labels: %v", err)
	}

	// override only a few clusterStartOpts from fetched opts
	startClusterOpts.HostAliases = fetchedClusterStartOpts.HostAliases

	if err != nil {
		log.SendCloudReport("error", "Failed getting cluster start options", "Failed",
			&map[string]interface{}{"error": err.Error()})
		return err
	}
	err = k3dCluster.ClusterStart(ctx, runtimes.SelectedRuntime, cluster, startClusterOpts)
	if err != nil {
		log.SendCloudReport("error", "Failed running cluster", "Failed",
			&map[string]interface{}{"error": err.Error()})
	}
	return err
}

func IsGpuCluster(cluster *Cluster) bool {
	return len(cluster.Nodes) > 0 && strings.Contains(cluster.Nodes[0].Image, "cuda")
}

func createClusterConfig(ctx context.Context, manifest *manifest.InstallationManifest, port uint, volumes []string, useGpu bool) *conf.ClusterConfig {
	freePort, err := cliutil.GetFreePort()
	if err != nil {
		log.Fatalln(err)
	}

	image := manifest.Images.K3s
	if useGpu {
		image = manifest.Images.K3sGpu
	}

	mirrorConfig, err := CreateMirrorFromManifest(manifest, fmt.Sprintf("http://%s", REGISTRY_DOMAIN))
	if err != nil {
		log.Fatalln(err)
	}

	simpleK3dConfig := conf.SimpleConfig{
		TypeMeta: k3dConfTypes.TypeMeta{
			Kind:       "Simple",
			APIVersion: "k3d.io/v1alpha5",
		},
		ObjectMeta: k3dConfTypes.ObjectMeta{
			Name: CLUSTER_NAME,
		},
		Servers: 1,
		ExposeAPI: conf.SimpleExposureOpts{
			HostPort: strconv.Itoa(freePort),
		},
		Image:   image,
		Volumes: make([]conf.VolumeWithNodeFilters, len(volumes)),
		Ports: []conf.PortWithNodeFilters{{
			Port:        fmt.Sprintf("%v:80", port),
			NodeFilters: []string{"server:*:direct"},
		}},
		Env: []conf.EnvVarWithNodeFilters{
			{
				EnvVar:      fmt.Sprintf("all_proxy=%s", os.Getenv("all_proxy")),
				NodeFilters: []string{"server:*"},
			},
			{
				EnvVar:      fmt.Sprintf("ALL_PROXY=%s", os.Getenv("ALL_PROXY")),
				NodeFilters: []string{"server:*"},
			},
			{
				EnvVar:      fmt.Sprintf("http_proxy=%s", os.Getenv("http_proxy")),
				NodeFilters: []string{"server:*"},
			},
			{
				EnvVar:      fmt.Sprintf("HTTP_PROXY=%s", os.Getenv("HTTP_PROXY")),
				NodeFilters: []string{"server:*"},
			},
			{
				EnvVar:      fmt.Sprintf("https_proxy=%s", os.Getenv("https_proxy")),
				NodeFilters: []string{"server:*"},
			},
			{
				EnvVar:      fmt.Sprintf("HTTPS_PROXY=%s", os.Getenv("HTTPS_PROXY")),
				NodeFilters: []string{"server:*"},
			},
			{
				EnvVar:      fmt.Sprintf("no_proxy=%s", os.Getenv("no_proxy")),
				NodeFilters: []string{"server:*"},
			},
			{
				EnvVar:      fmt.Sprintf("NO_PROXY=%s", os.Getenv("NO_PROXY")),
				NodeFilters: []string{"server:*"},
			},
		},
		Registries: conf.SimpleConfigRegistries{
			Use:    []string{"tensorleap-registry"},
			Config: mirrorConfig,
		},
		Options: conf.SimpleConfigOptions{
			K3dOptions: conf.SimpleConfigOptionsK3d{
				Wait:                true,
				DisableLoadbalancer: true,
			},
			K3sOptions: conf.SimpleConfigOptionsK3s{
				ExtraArgs: []conf.K3sArgWithNodeFilters{{
					Arg:         "--disable=traefik",
					NodeFilters: []string{"server:*"},
				}},
			},
			// Just for convenience to use kubectl, on install and upgrade we take the kubeconfig from the cluster
			KubeconfigOptions: conf.SimpleConfigOptionsKubeconfig{
				UpdateDefaultKubeconfig: true,
				SwitchCurrentContext:    true,
			},
		},
	}
	if useGpu {
		simpleK3dConfig.Options.Runtime.GPURequest = "all"
	}

	for i, v := range volumes {
		simpleK3dConfig.Volumes[i] = conf.VolumeWithNodeFilters{
			Volume:      v,
			NodeFilters: []string{"server:*"},
		}
	}

	k3dClusterConfig, err := config.TransformSimpleToClusterConfig(ctx, runtimes.SelectedRuntime, simpleK3dConfig)
	if err != nil {
		log.Fatalln(err)
	}

	k3dClusterConfig, err = config.ProcessClusterConfig(*k3dClusterConfig)
	if err != nil {
		log.Fatalln(err)
	}

	return k3dClusterConfig
}

func UninstallCluster(ctx context.Context) error {
	cluster, err := GetCluster(ctx)
	if err != nil {
		log.SendCloudReport("error", "Failed getting cluster", "Failed", &map[string]interface{}{"error": err.Error()})
		return err
	}
	if cluster == nil {
		log.SendCloudReport("info", "Cluster not found", "Running", nil)
		log.Info("Cluster 'tensorleap' not found")
		return nil
	}
	log.Info("Deleting cluster 'tensorleap'")
	opt := k3d.ClusterDeleteOpts{
		SkipRegistryCheck: true,
	}
	err = k3dCluster.ClusterDelete(ctx, runtimes.SelectedRuntime, cluster, opt)
	if err != nil {
		log.SendCloudReport("error", "Failed deleting cluster", "Failed",
			&map[string]interface{}{"opt": opt, "cluster": cluster, "runtime": runtimes.SelectedRuntime, "error": err.Error()})
	}
	return err
}

func FixDockerDns() {
	os.Setenv(k3d.K3dEnvFixDNS, "true")
}