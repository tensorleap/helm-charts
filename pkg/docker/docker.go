package docker

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/spf13/pflag"
	"github.com/tensorleap/helm-charts/pkg/log"
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
					errChan <- fmt.Errorf("failed to pull image %s after %d attempts: %w", imageName, maxRetries, err)
					cancel() // Cancel the context to stop ongoing pull operations
					breakLoop = true
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

	log.Info("Saving images...")
	out, err := dockerCli.ImageSave(context.Background(), imageNames)
	if err != nil {
		return err
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
