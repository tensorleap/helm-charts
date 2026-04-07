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
// checks the k3d node for the DiskPressure condition. When detected, it warns
// the user with actionable guidance. The monitor should be stopped by calling
// Stop() when helm operations complete.
func StartDiskPressureMonitor(kubeConfigPath, kubeContext string) (*DiskPressureMonitor, error) {
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

	go monitorDiskPressure(ctx, clientset, done)

	return &DiskPressureMonitor{cancel: cancel, done: done}, nil
}

func (m *DiskPressureMonitor) Stop() {
	m.cancel()
	<-m.done
}

func monitorDiskPressure(ctx context.Context, clientset kubernetes.Interface, done chan struct{}) {
	defer close(done)

	ticker := time.NewTicker(diskPressureCheckInterval)
	defer ticker.Stop()

	var lastWarnedAt time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !hasDiskPressure(ctx, clientset) {
				continue
			}

			now := time.Now()
			if lastWarnedAt.IsZero() || now.Sub(lastWarnedAt) >= diskPressureRepeatInterval {
				lastWarnedAt = now
				emitDiskPressureWarning()
			}
		}
	}
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

func emitDiskPressureWarning() {
	log.Warn("Disk pressure detected.")
	log.Warn("This may stall the installation.")
	log.Warnf("If installation does not complete within a few minutes, consider increasing the disk space available to docker so that it has at least %s free storage, and then retrying the installation.", RECOMMENDED_STORAGE_PRETTY)
	log.SendCloudReport("warning", "Disk pressure detected during installation", "Running", nil)
}
