package docker

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"sync"

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

func DownloadDockerImages(dockerCli Client, imageNames []string, outputFile io.Writer) error {
	wg := sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error)

	for _, imageName := range imageNames {
		wg.Add(1)
		go func(imageName string) {
			defer wg.Done()
			log.Println("Pulling image:", imageName)
			out, err := dockerCli.ImagePull(ctx, imageName, types.ImagePullOptions{})

			if err != nil {
				errChan <- err
				cancel() // Cancel the context to stop ongoing push operations
			}
			_, err = io.Copy(io.Discard, out)
			if err != nil {
				errChan <- err
				cancel() // Cancel the context to stop ongoing push operations
			}
			log.Println("Pulled", imageName)

		}(imageName)
	}

	wg.Wait()

	select {
	case err := <-errChan:
		return fmt.Errorf("push operations were stopped due to an error: %v", err)
	case <-ctx.Done():
		return fmt.Errorf("push operations were stopped due to an error: %v", ctx.Err())
	default:
		log.Printf("All images pulled successfully.\n")
	}

	log.Info("Saving images...")
	out, err := dockerCli.ImageSave(context.Background(), imageNames)
	if err != nil {
		return err
	}
	defer out.Close()

	if err != nil {
		return err
	}

	gzipWriter := gzip.NewWriter(outputFile)
	defer gzipWriter.Close()
	_, err = io.Copy(gzipWriter, out)
	if err != nil {
		return err
	}
	log.Info("Saved images")
	return nil
}
