package k3d

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	dockerTypes "github.com/docker/docker/api/types"
	cliutil "github.com/k3d-io/k3d/v5/cmd/util"
	"github.com/k3d-io/k3d/v5/pkg/client"
	"github.com/k3d-io/k3d/v5/pkg/runtimes"

	k3d "github.com/k3d-io/k3d/v5/pkg/types"
	"github.com/tensorleap/helm-charts/pkg/docker"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/utils"
)

type Registry = k3d.Registry

const (
	REQUIRED_MEMORY         = 6227000000
	REQUIRED_MEMORY_PRETTY  = "6Gb"
	REQUIRED_STORAGE_KB     = 16777216
	REQUIRED_STORAGE_PRETTY = "15Gb"
)

const (
	REGISTRY_NAME   = "k3d-tensorleap-registry"
	CONTAINER_NAME  = "k3d-tensorleap-server-0"
	REGISTRY_DOMAIN = "k3d-tensorleap-registry:5000"
	// the registry is limited so we can't push too many images in parallel
	MAX_CONCURRENT_CACHE_IMAGE = 2
	PUSH_IMAGE_RETRY           = 3
)

type RegistryTagListResponse struct {
	Name string
	Tags []string
}

func GetLocalRegistryPort(ctx context.Context) (string, error) {
	reg, err := client.RegistryGet(ctx, runtimes.SelectedRuntime, REGISTRY_NAME)
	if err != nil {
		log.SendCloudReport("error", "Failed getting local registry port", "Failed",
			&map[string]interface{}{"registryName": REGISTRY_NAME, "selectedRuntime": runtimes.SelectedRuntime, "error": err.Error()})
		return "", err
	}

	return GetRegistryPort(ctx, reg)
}

func GetRegistryPort(ctx context.Context, reg *Registry) (string, error) {
	registryNode, err := runtimes.SelectedRuntime.GetNode(ctx, &k3d.Node{Name: reg.Host})
	if err != nil {
		return "", err
	}

	regPort := registryNode.Ports["5000/tcp"][0].HostPort
	return regPort, nil
}

type CreateRegistryParams struct {
	Port    uint     `json:"port"`
	Volumes []string `json:"volumes"`
}

func CreateLocalRegistry(ctx context.Context, imageName string, params *CreateRegistryParams) (*Registry, error) {
	if existingRegistry, _ := client.RegistryGet(ctx, runtimes.SelectedRuntime, REGISTRY_NAME); existingRegistry != nil {
		log.Println("Found existing registry!")
		log.SendCloudReport("info", "Found existing registry", "Running", &map[string]interface{}{"registryName": REGISTRY_NAME, "existingRegistry": existingRegistry})

		return existingRegistry, nil
	}

	reg := createRegistryConfig(imageName, params)
	_, err := client.RegistryRun(ctx, runtimes.SelectedRuntime, reg)
	if err != nil {
		log.SendCloudReport("error", "Failed running k3d registry", "Failed",
			&map[string]interface{}{"registryName": REGISTRY_NAME, "selectedRuntime": runtimes.SelectedRuntime, "port": params.Port, "volumes": params.Volumes, "error": err.Error()})
		return nil, err
	}

	err = utils.WaitForCondition(func() (bool, error) {
		port := strconv.FormatUint(uint64(params.Port), 10)
		return IsRegistryReady(ctx, port)
	}, 5*time.Second, 2*time.Minute)
	if err != nil {
		log.SendCloudReport("error", "Failed waiting for registry to be ready", "Failed",
			&map[string]interface{}{"registryName": REGISTRY_NAME, "selectedRuntime": runtimes.SelectedRuntime, "port": params.Port, "volumes": params.Volumes, "error": err.Error()})
		return nil, err
	}

	log.SendCloudReport("info", "Successfully created k3d regisrty", "Running", &map[string]interface{}{"registryName": REGISTRY_NAME})
	return reg, nil
}

func createRegistryConfig(imageName string, params *CreateRegistryParams) *Registry {
	exposePort, err := cliutil.ParsePortExposureSpec(strconv.FormatUint(uint64(params.Port), 10), k3d.DefaultRegistryPort)
	if err != nil {
		log.SendCloudReport("error", "Failed creating k3d registry config", "Failed",
			&map[string]interface{}{"defaultRegistry": k3d.DefaultRegistryPort, "port": params.Port, "exposedPort": exposePort, "error": err.Error()})
		log.Fatalln(err)
	}

	reg := &Registry{
		Host:         REGISTRY_NAME,
		Image:        imageName,
		ExposureOpts: *exposePort,
		Network:      k3d.DefaultRuntimeNetwork,
		Volumes:      params.Volumes,
	}

	return reg
}

func UninstallRegister() error {
	ctx := context.Background()
	existingRegistry, _ := client.RegistryGet(ctx, runtimes.SelectedRuntime, REGISTRY_NAME)
	if existingRegistry == nil {
		log.Infof("Registry '%s' not found", REGISTRY_NAME)
		log.SendCloudReport("info", "Not found registry", "Running", &map[string]interface{}{"registryName": REGISTRY_NAME})

		return nil
	}
	log.Infof("Deleting registry %s", REGISTRY_NAME)

	node, err := client.NodeGet(ctx, runtimes.SelectedRuntime, &k3d.Node{Name: REGISTRY_NAME})
	if err != nil {
		return fmt.Errorf("failed to get node: %w", err)
	}

	err = client.NodeDelete(ctx, runtimes.SelectedRuntime, node, k3d.NodeDeleteOpts{SkipLBUpdate: true})
	if err != nil {
		return fmt.Errorf("error removing the registry container: %v", err)
	}
	log.SendCloudReport("info", "Registry removed successfully", "Running", &map[string]interface{}{"registryName": REGISTRY_NAME})
	return nil
}

func IsRegistryReady(ctx context.Context, regPort string) (bool, error) {
	_, err := client.RegistryGet(ctx, runtimes.SelectedRuntime, REGISTRY_NAME)
	if err != nil {
		return false, fmt.Errorf("failed to check is registry ready: %s", err)
	}
	url := fmt.Sprintf("http://127.0.0.1:%s/v2/_catalog", regPort)
	_, err = http.Get(url)
	return err == nil, nil
}

func isImageInRegistry(ctx context.Context, image string, regPort string) (bool, error) {
	imageParts := strings.SplitN(image, ":", 2)
	imageTag := imageParts[1]
	urlLength := strings.IndexRune(imageParts[0], '/')
	imageFullPath := imageParts[0][urlLength:]
	tagsListUrl := fmt.Sprintf("http://127.0.0.1:%s/v2%s/tags/list", regPort, imageFullPath)

	resp, err := http.Get(tagsListUrl)
	if err != nil {
		// In arirgap mode, the registry will return EOF when the image is not found
		if strings.HasSuffix(err.Error(), "EOF") {
			return false, nil
		}
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("local registry returned bad status code: %v", resp.StatusCode)
	}

	tagsListRaw, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	tagsList := RegistryTagListResponse{}
	if err = json.Unmarshal(tagsListRaw, &tagsList); err != nil {
		return false, err
	}

	for _, tag := range tagsList.Tags {
		if tag == imageTag {
			return true, nil
		}
	}

	return false, nil
}

func PullingImage(ctx context.Context, dockerClient docker.Client, image string) error {

	resp, err := dockerClient.ImagePull(ctx, image, dockerTypes.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("docker failed to pull the image '%s': %w", image, err)
	}
	defer resp.Close()
	log.Infof("Pulling image '%s'\n", image)

	// this prints out status of the pull, consider having that under some flags or doing fancy stuff to display it
	// _, err = io.Copy(os.Stdout, resp)
	_, err = io.Copy(io.Discard, resp)
	if err != nil {
		log.Warnf("Couldn't get docker output: %v", err)
	}
	return nil
}

func CacheImage(ctx context.Context, dockerClient docker.Client, image string, regPort string) error {
	targetImage := fmt.Sprintf(
		"127.0.0.1:%s%s",
		regPort,
		strings.TrimLeftFunc(image, func(r rune) bool {
			return r != '/'
		}),
	)

	if err := dockerClient.ImageTag(ctx, image, targetImage); err != nil {
		return err
	}

	retry := PUSH_IMAGE_RETRY
	for {
		resp, err := dockerClient.ImagePush(ctx, targetImage, dockerTypes.ImagePushOptions{
			RegistryAuth: "empty auth",
		})
		if err != nil {
			return fmt.Errorf("docker failed to push the image '%s': %w", targetImage, err)
		}
		defer resp.Close()

		log.Infof("Pushing image '%s'\n", targetImage)

		pushImageOutput, err := io.ReadAll(resp)
		if err != nil {
			return fmt.Errorf("couldn't get docker output: %v", err)
		}

		imageAlreadyInRegistry, err := isImageInRegistry(ctx, image, regPort)
		if err != nil {
			return fmt.Errorf("failed to re-check image(%s) existents", image)
		}

		if !imageAlreadyInRegistry {

			if retry > 0 {
				retry--
				time.Sleep(10 * time.Second)
				log.Warnf("Retry to push image %s ", image)
				continue
			}
			log.Warnf("Failed to push image (%s), see docker output bellow:", image)
			log.Info(string(pushImageOutput))
			return fmt.Errorf("check docker output above, consider to try again on connection error")
		}
		log.Printf("Pushed image '%s'\n", targetImage)
		break
	}

	return nil
}

func CacheImagesInParallel(ctx context.Context, images []string, regPort string, isAirgap bool) error {
	imagesNotInRegistry := []string{}
	for _, img := range images {
		imageInRegistry, err := isImageInRegistry(ctx, img, regPort)
		if err != nil {
			return fmt.Errorf("failed to check if image %s is in registry: %s", img, err)
		}
		if !imageInRegistry {
			imagesNotInRegistry = append(imagesNotInRegistry, img)
		} else {
			log.Infof("Image already cached '%s'\n", img)
		}
	}

	dockerClient, err := docker.NewClient()
	if err != nil {
		return err
	}
	if !isAirgap && len(imagesNotInRegistry) > 0 {
		log.Info("Downloading docker images...")
		wg := sync.WaitGroup{}
		for _, img := range imagesNotInRegistry {
			wg.Add(1)
			go func(img string) {
				defer wg.Done()
				if err := PullingImage(ctx, dockerClient, img); err != nil {
					log.SendCloudReport("error", "Failed pulling image", "Failed", &map[string]interface{}{"image": img, "error": err.Error()})
					log.Fatalf("Failed to pull %s: %s", img, err)
				}
			}(img)
		}
		wg.Wait()
	}

	tm := utils.NewTaskManager(MAX_CONCURRENT_CACHE_IMAGE)
	for _, img := range imagesNotInRegistry {
		tm.Add()
		go func(img string) {
			defer tm.Done()
			if err := CacheImage(ctx, dockerClient, img, regPort); err != nil {
				log.SendCloudReport("error", "Failed caching image", "Failed", &map[string]interface{}{"image": img, "error": err.Error()})
				log.Fatalf("failed to cache %s: %s", img, err)
			}
		}(img)
	}
	tm.Wait()
	log.SendCloudReport("info", "Successfully cached images in parallel", "Running", nil)
	return nil
}

func CacheImageInTheBackground(ctx context.Context, image string) error {
	regPort, err := GetLocalRegistryPort(ctx)
	if err != nil {
		return err
	}
	imageAlreadyInRegistry, err := isImageInRegistry(ctx, image, regPort)
	if err != nil {
		return err
	}

	dockerClient, err := docker.NewClient()
	if err != nil {
		log.SendCloudReport("error", "Failed fetching docker client", "Failed", &map[string]interface{}{"error": err.Error()})
		return err
	}

	urlLength := strings.IndexRune(image, '/')
	targetImage := REGISTRY_DOMAIN + image[urlLength:]

	shellScript := fmt.Sprintf("crictl pull %s", image)
	if !imageAlreadyInRegistry {
		shellScript = strings.Join([]string{
			shellScript,
			fmt.Sprintf("ctr image convert %s %s", image, targetImage),
			fmt.Sprintf("ctr image push --plain-http %s", targetImage),
		}, " && ")
	}
	exec, err := dockerClient.ContainerExecCreate(ctx, CONTAINER_NAME, dockerTypes.ExecConfig{
		Privileged: true,
		Detach:     true,
		Cmd:        []string{"sh", "-c", shellScript},
	})
	if err != nil {
		log.SendCloudReport("error", "Failed creating exec config for node", "Failed",
			&map[string]interface{}{"containerName": CONTAINER_NAME, "error": err.Error()})
		return fmt.Errorf("docker failed to create exec config for node '%s': %w", CONTAINER_NAME, err)
	}

	log.SendCloudReport("info", "Successfully cached images in background", "Running", nil)
	return dockerClient.ContainerExecStart(ctx, exec.ID, dockerTypes.ExecStartCheck{})
}

func CheckDockerRequirements(checkDockerRequirementImage string, isAirgap bool) error {
	if os.Getenv("DISABLE_DOCKER_CHECKS") == "true" {
		return nil
	}
	_, err := exec.LookPath("docker")
	if err != nil {
		return errors.New("docker is not installed. docker is prerequisite, please install it and retry. https://docs.docker.com/engine/install/")
	}

	cmd := exec.Command("docker", "ps")
	err = cmd.Run()
	if err != nil {
		return errors.New("docker is not running")
	}

	log.Println("Checking docker memory limits...")
	dockerClient, err := docker.NewClient()
	if err != nil {
		return fmt.Errorf("docker failed to get client: %w", err)
	}
	dockerInfo, err := dockerClient.Info(context.Background())
	if err != nil {
		log.Fatalf("Failed getting docker info, %s", err)
		return err
	}
	dockerMemoryPretty := fmt.Sprintf("%dGb", dockerInfo.MemTotal/(1024*1024*1024))
	log.Printf("Docker has %s memory available.\n", dockerMemoryPretty)

	log.Println("Checking docker storage limits...")

	if !isAirgap {
		_, err = dockerClient.ImagePull(context.Background(), checkDockerRequirementImage, dockerTypes.ImagePullOptions{})
		if err != nil {
			log.Fatalf("Failed pulling  %s image, %s", checkDockerRequirementImage, err)
		}
	}

	runCmdStr := fmt.Sprintf("docker run --rm %s df -t overlay -P", checkDockerRequirementImage)
	cmd = exec.Command("sh", "-c", runCmdStr)
	dfOutputBytes, err := cmd.Output()
	if err != nil {
		log.Fatalf("Failed pulling %s, %s", checkDockerRequirementImage, err)
		return err
	}
	// the output looks like this:
	// Filesystem           1024-blocks    Used Available Capacity Mounted on
	// overlay              345672852  98074428 229966016  30% /
	dfOutput := string(dfOutputBytes)
	dfOutputLines := strings.Split(dfOutput, "\n")
	dfOutputWords := strings.Fields(dfOutputLines[1])
	dockerTotalStorageKB, _ := strconv.Atoi(dfOutputWords[1])
	dockerTotalStoragePretty := fmt.Sprintf("%dGb", dockerTotalStorageKB/(1024*1024))
	dockerFreeStorageKB, _ := strconv.Atoi(dfOutputWords[3])
	dockerFreeStoragePretty := fmt.Sprintf("%dGb", dockerFreeStorageKB/(1024*1024))
	log.Printf("Docker has %s free storage available (%s total).\n", dockerFreeStoragePretty, dockerTotalStoragePretty)
	var noResources bool

	if dockerInfo.MemTotal < int64(REQUIRED_MEMORY) {
		log.Printf("Please increase docker memory limit to at least %s\n", REQUIRED_MEMORY_PRETTY)
		noResources = true
	}

	if dockerFreeStorageKB < REQUIRED_STORAGE_KB {
		log.Printf("Please increase docker storage limit, tensorleap required at least %s free storage\n", REQUIRED_STORAGE_PRETTY)
		noResources = true
	}

	if noResources {
		log.Println("Please retry installation after updating your docker config.")
		return errors.New("not enough resources")
	}

	return nil
}
