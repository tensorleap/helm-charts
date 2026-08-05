package k3d

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/tensorleap/helm-charts/pkg/log"
)

const (
	corednsCpu    = "250m"
	corednsMemory = "512Mi"
)

// PatchCoreDnsResources sets requests==limits on CoreDNS so it runs as Guaranteed QoS:
// the k3s default (70Mi request / 170Mi limit, Burstable) both OOM-kills CoreDNS under
// the engine's DNS query storms and makes it the kernel OOM killer's first victim under
// node memory pressure (oom_score_adj ~999 vs -997 for Guaranteed). k3s only re-applies
// its packaged coredns manifest when the k3s version changes, which happens exclusively
// through install/upgrade — so re-patching here keeps the change durable.
func PatchCoreDnsResources(ctx context.Context, kubeConfigPath, kubeContext string) error {
	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeConfigPath},
		&clientcmd.ConfigOverrides{CurrentContext: kubeContext},
	).ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig for coredns patch: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client for coredns patch: %w", err)
	}

	patch := fmt.Sprintf(`{"spec":{"template":{"spec":{"containers":[{"name":"coredns","resources":{"requests":{"cpu":"%s","memory":"%s"},"limits":{"cpu":"%s","memory":"%s"}}}]}}}}`,
		corednsCpu, corednsMemory, corednsCpu, corednsMemory)

	_, err = clientset.AppsV1().Deployments("kube-system").Patch(
		ctx, "coredns", types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("failed to patch coredns resources: %w", err)
	}

	log.Infof("Patched coredns resources to Guaranteed QoS (cpu %s, memory %s)", corednsCpu, corednsMemory)
	return nil
}
