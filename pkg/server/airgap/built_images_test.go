package airgap

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCollectBuiltImages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/tensorleap/engine-generic/tags/list" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, `{"name":"tensorleap/engine-generic","tags":["e2f65ffd695826db0231b4f05edfd7b771672b50","buildcache","master-2c133e0e-py38","feature-fork-workers-8cad7062-py38","24a6b90708a29fba2c1c35d6db96caa3c7366008"]}`)
	}))
	defer server.Close()

	registry := strings.TrimPrefix(server.URL, "http://")
	images := CollectBuiltImages(registry)

	expected := []string{
		registry + "/tensorleap/engine-generic:e2f65ffd695826db0231b4f05edfd7b771672b50",
		registry + "/tensorleap/engine-generic:24a6b90708a29fba2c1c35d6db96caa3c7366008",
	}
	if len(images) != len(expected) {
		t.Fatalf("expected %d images, got %d: %v", len(expected), len(images), images)
	}
	for i, img := range expected {
		if images[i] != img {
			t.Errorf("expected %s, got %s", img, images[i])
		}
	}

	if got := CollectBuiltImages("127.0.0.1:1"); got != nil {
		t.Errorf("expected nil for unreachable registry, got %v", got)
	}
}
