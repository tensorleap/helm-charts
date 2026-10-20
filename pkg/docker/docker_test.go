package docker

import (
	"bytes"
	"context"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

// fakePullClient is a docker client that pulls, tags, lists and saves in memory.
// Only those methods are implemented; anything else panics.
type fakePullClient struct {
	client.APIClient

	mu       sync.Mutex
	failWith string // when set, every pull reports this error inside the stream
	pulled   []string
	tagged   [][2]string
	saved    []string
	present  map[string]bool // the RepoTags docker would list
}

func newFakePullClient() *fakePullClient {
	return &fakePullClient{present: map[string]bool{}}
}

func (f *fakePullClient) ImagePull(_ context.Context, ref string, _ image.PullOptions) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pulled = append(f.pulled, ref)
	if f.failWith != "" {
		// What docker sends when the registry refuses: HTTP 200, error in the stream.
		return io.NopCloser(strings.NewReader(
			`{"status":"Pulling from x"}` + "\n" +
				`{"errorDetail":{"message":"` + f.failWith + `"},"error":"` + f.failWith + `"}` + "\n")), nil
	}
	f.present[trimDefaultRegistry(ref)] = true
	return io.NopCloser(strings.NewReader(`{"status":"Pull complete"}` + "\n")), nil
}

func (f *fakePullClient) ImageTag(_ context.Context, source, target string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tagged = append(f.tagged, [2]string{source, target})
	f.present[trimDefaultRegistry(target)] = true
	return nil
}

func (f *fakePullClient) ImageList(context.Context, image.ListOptions) ([]image.Summary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	tags := []string{}
	for tag := range f.present {
		tags = append(tags, tag)
	}
	return []image.Summary{{RepoTags: tags}}, nil
}

func (f *fakePullClient) ImageSave(_ context.Context, names []string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saved = append(f.saved, names...)
	return io.NopCloser(strings.NewReader("tar")), nil
}

func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("DOCKER_CONFIG", t.TempDir()) // no real credentials in tests
	old := retryDelay
	retryDelay = 0
	t.Cleanup(func() { retryDelay = old })
}

func TestPullStreamError(t *testing.T) {
	tests := []struct {
		name    string
		stream  string
		wantErr string
	}{
		{"clean pull", `{"status":"Pulling fs layer"}` + "\n" + `{"status":"Pull complete"}`, ""},
		{"error detail", `{"status":"x"}` + "\n" + `{"errorDetail":{"message":"toomanyrequests: Data limit exceeded"},"error":"toomanyrequests: Data limit exceeded"}`, "toomanyrequests: Data limit exceeded"},
		{"error only", `{"error":"manifest unknown"}`, "manifest unknown"},
		{"not json", `<html>`, "reading the pull output"},
		{"empty", ``, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pullStreamError(strings.NewReader(tt.stream))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

// The 1.6.100 airgap pack logged "Pulled" for an image docker never pulled,
// because the error inside the stream was thrown away.
func TestPullDockerImagesFailsOnAnErrorInsideThePullStream(t *testing.T) {
	isolate(t)
	fake := newFakePullClient()
	fake.failWith = "toomanyrequests: Data limit exceeded"

	err := PullDockerImages(fake, []string{"public.ecr.aws/tensorleap/engine:1.6.100-f8e98ce1"})

	if err == nil || !strings.Contains(err.Error(), "Data limit exceeded") {
		t.Fatalf("error = %v, want the error from the stream", err)
	}
	if len(fake.pulled) != maxRetries {
		t.Errorf("pulled %d times, want %d attempts", len(fake.pulled), maxRetries)
	}
}

func TestDownloadDockerImages(t *testing.T) {
	const (
		firstParty = "public.ecr.aws/tensorleap/engine:1.6.100-f8e98ce1"
		vendor     = "docker.io/rancher/mirrored-pause:3.6"
		mirrored   = "public.ecr.aws/tensorleap/mirror/docker.io/rancher/mirrored-pause:3.6"
	)
	images := []string{firstParty, vendor}

	t.Run("pulls each image as named without pullFrom", func(t *testing.T) {
		isolate(t)
		fake := newFakePullClient()
		if err := DownloadDockerImages(fake, images, &bytes.Buffer{}, nil); err != nil {
			t.Fatal(err)
		}
		slices.Sort(fake.pulled)
		if want := []string{vendor, firstParty}; !slices.Equal(fake.pulled, want) {
			t.Errorf("pulled %v, want %v", fake.pulled, want)
		}
		if len(fake.tagged) != 0 {
			t.Errorf("tagged %v, want nothing", fake.tagged)
		}
	})

	t.Run("pulls from pullFrom and saves under the original names", func(t *testing.T) {
		isolate(t)
		fake := newFakePullClient()
		pullFrom := func(image string) string {
			if image == vendor {
				return mirrored
			}
			return image
		}
		if err := DownloadDockerImages(fake, images, &bytes.Buffer{}, pullFrom); err != nil {
			t.Fatal(err)
		}
		slices.Sort(fake.pulled)
		if want := []string{firstParty, mirrored}; !slices.Equal(fake.pulled, want) {
			t.Errorf("pulled %v, want %v (never the vendor)", fake.pulled, want)
		}
		if want := [][2]string{{mirrored, vendor}}; !slices.Equal(fake.tagged, want) {
			t.Errorf("tagged %v, want %v", fake.tagged, want)
		}
		if !slices.Equal(fake.saved, images) {
			t.Errorf("saved %v, want the original names %v", fake.saved, images)
		}
	})
}
