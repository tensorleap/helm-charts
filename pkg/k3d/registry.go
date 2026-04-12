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
	"time"

	dockerimage "github.com/docker/docker/api/types/image"
	"github.com/tensorleap/helm-charts/pkg/docker"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/utils"
)

var (
	REQUIRED_MEMORY         int64 = 6227000000
	REQUIRED_MEMORY_PRETTY        = "6Gb"
	REQUIRED_STORAGE_KB     int64
	REQUIRED_STORAGE_PRETTY string
)

func init() {
	REQUIRED_STORAGE_KB = INSTALLATION_STORAGE_REQUIRED_GB * 1024 * 1024
	REQUIRED_STORAGE_PRETTY = fmt.Sprintf("%dGb", INSTALLATION_STORAGE_REQUIRED_GB)
}

const (
	CONTAINER_NAME             = "k3d-tensorleap-server-0"
	MAX_CONCURRENT_CACHE_IMAGE = 2
	PUSH_IMAGE_RETRY           = 3
)

type RegistryTagListResponse struct {
	Name string
	Tags []string
}

// IsRegistryReady checks if the Zot registry is reachable at the given port
func IsRegistryReady(ctx context.Context, regPort string) (bool, error) {
	url := fmt.Sprintf("http://127.0.0.1:%s/v2/", regPort)
	resp, err := http.Get(url)
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200, nil
}

// WaitForRegistry waits for the in-cluster Zot registry to become reachable
func WaitForRegistry(ctx context.Context, regPort string) error {
	return utils.WaitForCondition(func() (bool, error) {
		return IsRegistryReady(ctx, regPort)
	}, 5*time.Second, 3*time.Minute)
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

const maxRetries = 3
const retryDelay = 2 * time.Second

func PullingImage(ctx context.Context, dockerClient docker.Client, image string) error {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Printf("Pulling image '%s' (attempt %d/%d)\n", image, attempt, maxRetries)
		resp, err := dockerClient.ImagePull(ctx, image, dockerimage.PullOptions{})
		if err == nil {
			defer resp.Close() // Ensure the response is closed to avoid resource leaks

			// Discard the output or handle it as needed
			_, err = io.Copy(io.Discard, resp)
			if err == nil {
				log.Printf("Successfully pulled image '%s'\n", image)
				return nil
			}

			// If we encounter an error after successfully pulling, handle it here
			log.Printf("Failed to read response for image '%s': %v\n", image, err)
			lastErr = err
		} else {
			log.Printf("Failed to pull image '%s': %v\n", image, err)
			lastErr = err
		}

		if attempt < maxRetries {
			log.Printf("Retrying in %s...\n", retryDelay)
			time.Sleep(retryDelay)
		}
	}

	// If all attempts fail, return the last encountered error
	return fmt.Errorf("failed to pull image '%s' after %d attempts: %w", image, maxRetries, lastErr)
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
		resp, err := dockerClient.ImagePush(ctx, targetImage, dockerimage.PushOptions{
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

func CacheImagesInParallel(ctx context.Context, images []string, regPort string, isAirgap bool, imageCachingMethod string) error {

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
		if err := docker.PullDockerImages(dockerClient, imagesNotInRegistry); err != nil {
			return fmt.Errorf("failed to pull images: %w", err)
		}
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
		_, err = dockerClient.ImagePull(context.Background(), checkDockerRequirementImage, dockerimage.PullOptions{})
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

	if dockerInfo.MemTotal < REQUIRED_MEMORY {
		log.Printf("Please increase docker memory limit to at least %s\n", REQUIRED_MEMORY_PRETTY)
		noResources = true
	}

	if int64(dockerFreeStorageKB) < REQUIRED_STORAGE_KB {
		log.Printf("Please increase docker storage limit, tensorleap requires at least %s free storage\n", REQUIRED_STORAGE_PRETTY)
		noResources = true
	}

	if noResources {
		log.Println("Please retry installation after updating your docker config.")
		return errors.New("not enough resources")
	}

	return nil
}
