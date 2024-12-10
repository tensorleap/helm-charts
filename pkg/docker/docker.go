package docker

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/spf13/pflag"
	"github.com/tensorleap/helm-charts/pkg/log"
	"k8s.io/utils/strings/slices"
)

type Client = client.APIClient

// this is a copy of the function from github.com/k3d-io/k3d/v5/pkg/runtimes/docker/util.go but with our log level
func NewClient() (Client, error) {
	dockerCli, err := command.NewDockerCli(command.WithStandardStreams())
	if err != nil {
		return nil, fmt.Errorf("failed to create new docker CLI with standard streams: %w", err)
	}

	newClientOpts := flags.NewClientOptions()
	newClientOpts.LogLevel = log.GetLevel().String() // this is needed, as the following Initialize() call will set a new log level on the global logrus instance

	flagset := pflag.NewFlagSet("docker", pflag.ContinueOnError)
	newClientOpts.InstallFlags(flagset)
	newClientOpts.SetDefaultOptions(flagset)

	err = dockerCli.Initialize(newClientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize docker CLI: %w", err)
	}

	return dockerCli.Client(), nil
}

func LoadingImages(dockerClient Client, reader io.Reader) error {
	log.Info("Loading images...")
	res, err := dockerClient.ImageLoad(context.Background(), reader, false)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	_, err = io.Copy(log.VerboseLogger.Out, res.Body)
	if err != nil {
		return err
	}
	log.Info("Images loaded")

	return nil
}

const maxRetries = 3
const retryDelay = 2 * time.Second

func DownloadDockerImages(dockerCli Client, imageNames []string, outputFile io.Writer) error {
	wg := sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error)
	breakLoop := false

	for _, imageName := range imageNames {
		if breakLoop {
			break
		}
		wg.Add(1)
		cancelWithError := func(err error) {
			errChan <- err
			cancel()
			breakLoop = true
		}
		go func(imageName string) {
			defer wg.Done()

			for attempt := 1; attempt <= maxRetries; attempt++ {
				if ctx.Err() != nil {
					breakLoop = true
					break
				}

				log.Printf("Pulling image: %s (attempt %d/%d)\n", imageName, attempt, maxRetries)
				out, err := dockerCli.ImagePull(ctx, imageName, types.ImagePullOptions{})
				if err == nil {
					defer out.Close() // Ensure the output stream is closed

					_, err = io.Copy(io.Discard, out)
					if err == nil {
						log.Println("Pulled", imageName)
						break
					}
				}

				log.Printf("Failed to pull image: %s (attempt %d/%d), error: %v\n", imageName, attempt, maxRetries, err)
				if attempt < maxRetries {
					time.Sleep(retryDelay)
				} else {
					cancelWithError(fmt.Errorf("failed to pull image %s after %d attempts: %w", imageName, maxRetries, err))
					break
				}
			}
		}(imageName)
	}

	wg.Wait()

	select {
	case err := <-errChan:
		return fmt.Errorf("pull operations were stopped due to an error: %v", err)
	default:
		log.Println("All images pulled successfully.")
	}

	err := EnsureImagesExists(dockerCli, imageNames)
	if err != nil {
		return err
	}

	log.Info("Saving images...")
	out, err := dockerCli.ImageSave(context.Background(), imageNames)
	if err != nil {
		return fmt.Errorf("it appears that some images failed to pull: %v", err)
	}
	defer out.Close()

	gzipWriter := gzip.NewWriter(outputFile)
	defer gzipWriter.Close()
	_, err = io.Copy(gzipWriter, out)
	if err != nil {
		return err
	}

	log.Info("Saved images")
	return nil
}

func trimDefaultRegistry(imageName string) string {
	if strings.HasPrefix(imageName, "docker.io/library/") {
		return strings.TrimPrefix(imageName, "docker.io/library/")
	}
	if strings.HasPrefix(imageName, "docker.io/") {
		return strings.TrimPrefix(imageName, "docker.io/")
	}
	return imageName
}

func GetExistedAndNotExistedImages(dockerCli client.APIClient, imageNames []string) (foundImages []string, notFoundImages []string, err error) {
	var allLocalImages []types.ImageSummary
	allLocalImages, err = dockerCli.ImageList(context.Background(), types.ImageListOptions{})
	if err != nil {
		err = fmt.Errorf("error listing Docker images: %v", err)
		return
	}

outer:
	for _, imageName := range imageNames {
		trimImageName := trimDefaultRegistry(imageName)
		for _, image := range allLocalImages {
			if slices.Contains(image.RepoTags, trimImageName) {
				foundImages = append(foundImages, imageName)
				continue outer
			}
		}
		notFoundImages = append(notFoundImages, imageName)
	}

	return
}

func EnsureImagesExists(dockerCli client.APIClient, imageNames []string) error {

	_, notFoundImages, err := GetExistedAndNotExistedImages(dockerCli, imageNames)

	if err != nil {
		return err
	}
	if len(notFoundImages) > 0 {
		return fmt.Errorf("images not found: %v", notFoundImages)
	}
	return nil
}

func RemoveImages(dockerCli client.APIClient, imageNames []string) error {
	existedImages, _, err := GetExistedAndNotExistedImages(dockerCli, imageNames)
	if err != nil {
		return err
	}

	for _, imageName := range existedImages {

		log.Printf("Removing image: %s\n", imageName)

		_, err := dockerCli.ImageRemove(context.Background(), imageName, types.ImageRemoveOptions{})
		if err != nil {
			log.Warnf("failed to remove image %s: %v", imageName, err)
		}
	}
	return nil
}
