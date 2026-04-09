package k3d

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/tensorleap/helm-charts/pkg/log"
)

const (
	diskPressureCheckInterval  = 15 * time.Second
	diskPressureRepeatInterval = 2 * time.Minute
)

type DiskPressureMonitor struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// StartDiskPressureMonitor begins a background goroutine that periodically
// checks the k3d node for the DiskPressure condition. When detected, it
// estimates the required storage from image manifests (once), then warns the
// user with actionable guidance. The monitor should be stopped by calling
// Stop() when helm operations complete.
func StartDiskPressureMonitor(kubeConfigPath, kubeContext string, allImages []string) (*DiskPressureMonitor, error) {
	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeConfigPath},
		&clientcmd.ConfigOverrides{CurrentContext: kubeContext},
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig for disk-pressure monitor: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client for disk-pressure monitor: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go monitorDiskPressure(ctx, clientset, allImages, done)

	return &DiskPressureMonitor{cancel: cancel, done: done}, nil
}

func (m *DiskPressureMonitor) Stop() {
	m.cancel()
	<-m.done
}

func monitorDiskPressure(ctx context.Context, clientset kubernetes.Interface, allImages []string, done chan struct{}) {
	defer close(done)

	ticker := time.NewTicker(diskPressureCheckInterval)
	defer ticker.Stop()

	var lastWarnedAt time.Time
	var recommendedStorage string
	estimated := false

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !hasDiskPressure(ctx, clientset) {
				continue
			}

			if !estimated {
				recommendedStorage = computeRecommendedStorage(allImages)
				estimated = true
			}

			now := time.Now()
			if lastWarnedAt.IsZero() || now.Sub(lastWarnedAt) >= diskPressureRepeatInterval {
				lastWarnedAt = now
				emitDiskPressureWarning(recommendedStorage)
			}
		}
	}
}

// computeRecommendedStorage queries image registries to estimate the required
// Docker storage. Falls back to the static RECOMMENDED_STORAGE_PRETTY if
// estimation fails.
func computeRecommendedStorage(allImages []string) string {
	if len(allImages) == 0 {
		return RECOMMENDED_STORAGE_PRETTY
	}

	log.Println("Estimating installation size from image manifests...")
	estimateStart := time.Now()
	estimatedBytes, err := EstimateInstallSize(allImages)
	log.Printf("Image size estimation took %s", time.Since(estimateStart).Round(time.Millisecond))
	if err != nil {
		log.Warnf("Could not estimate installation size: %v; using static recommendation", err)
		return RECOMMENDED_STORAGE_PRETTY
	}

	estimatedGB := estimatedBytes / (1024 * 1024 * 1024)
	requiredGB := estimatedGB + STORAGE_EVICTION_THRESHOLD_GB + storageBufferGB
	return fmt.Sprintf("%dGb", requiredGB)
}

func hasDiskPressure(ctx context.Context, clientset kubernetes.Interface) bool {
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return false
	}

	for _, node := range nodes.Items {
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeDiskPressure && condition.Status == corev1.ConditionTrue {
				return true
			}
		}
	}

	return false
}

func emitDiskPressureWarning(recommendedStorage string) {
	log.Warn("Disk pressure detected.")
	log.Warn("This may stall the installation.")
	log.Warnf("If installation does not complete within a few minutes, consider increasing the disk space available to docker so that it has at least %s free storage, and then retrying the installation.", recommendedStorage)
	log.SendCloudReport("warning", "Disk pressure detected during installation", "Running", nil)
}
