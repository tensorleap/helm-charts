package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/tensorleap/helm-charts/pkg/k3d"
)

const (
	keycloakPodSelector = "app.kubernetes.io/name=keycloakx"
	keycloakRealm       = "tensorleap"
	tempPasswordLength  = 16
)

// ResetPassword sets a temporary Keycloak password for the user with the given
// email by running kcadm inside the Keycloak pod, revokes the user's sessions
// and returns the password. Keycloak forces a new password at the next login.
func ResetPassword(ctx context.Context, email, password string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !strings.Contains(email, "@") {
		return "", fmt.Errorf("invalid email %q", email)
	}
	if password == "" {
		var err error
		if password, err = randomPassword(tempPasswordLength); err != nil {
			return "", err
		}
	}

	cfg, err := keycloakRestConfig(ctx)
	if err != nil {
		return "", err
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return "", fmt.Errorf("creating kubernetes client: %w", err)
	}
	pods, err := clientset.CoreV1().Pods(KUBE_NAMESPACE).List(ctx, metav1.ListOptions{
		LabelSelector: keycloakPodSelector,
		FieldSelector: "status.phase=Running",
	})
	if err != nil {
		return "", fmt.Errorf("listing keycloak pods in namespace %s: %w", KUBE_NAMESPACE, err)
	}
	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no running keycloak pod in namespace %s (is the server running with auth enabled?)", KUBE_NAMESPACE)
	}
	pod := pods.Items[0]

	req := clientset.CoreV1().RESTClient().Post().Resource("pods").Namespace(KUBE_NAMESPACE).
		Name(pod.Name).SubResource("exec").VersionedParams(&corev1.PodExecOptions{
		Container: pod.Spec.Containers[0].Name,
		Command:   []string{"bash", "-s"},
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
	}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("creating exec into pod %s: %w", pod.Name, err)
	}
	var stdout, stderr bytes.Buffer
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  strings.NewReader(resetPasswordScript(email, password)),
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return "", fmt.Errorf("resetting password for %s in pod %s: %w: %s", email, pod.Name, err, strings.TrimSpace(stderr.String()))
	}
	return password, nil
}

func keycloakRestConfig(ctx context.Context) (*rest.Config, error) {
	if os.Getenv("KUBECONFIG") != "" {
		return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			clientcmd.NewDefaultClientConfigLoadingRules(),
			&clientcmd.ConfigOverrides{},
		).ClientConfig()
	}
	path, err := k3d.ResolveSharedKubeConfig(ctx)
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, fmt.Errorf("tensorleap cluster not found, run `leap server run` first")
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: path},
		&clientcmd.ConfigOverrides{CurrentContext: KUBE_CONTEXT},
	).ClientConfig()
}

func randomPassword(n int) (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	out := make([]byte, n)
	for i := range out {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", fmt.Errorf("generating password: %w", err)
		}
		out[i] = alphabet[idx.Int64()]
	}
	return string(out), nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func resetPasswordScript(email, password string) string {
	return fmt.Sprintf(`set -euo pipefail
KC=/opt/keycloak/bin/kcadm.sh
CFG=/tmp/kcadm-$$.config
trap 'rm -f "$CFG"' EXIT
"$KC" config credentials --config "$CFG" --server http://localhost:8080/auth --realm master --user "${KC_BOOTSTRAP_ADMIN_USERNAME:-$KEYCLOAK_ADMIN}" --password "${KC_BOOTSTRAP_ADMIN_PASSWORD:-$KEYCLOAK_ADMIN_PASSWORD}" >/dev/null 2>&1
ID=$("$KC" get users --config "$CFG" -r %[3]s -q email=%[1]s -q exact=true --fields id --format csv --noquotes)
[ -n "$ID" ] || { echo "no user with email %[1]s in realm %[3]s" >&2; exit 2; }
[[ "$ID" != *$'\n'* ]] || { echo "more than one user matches %[1]s" >&2; exit 3; }
"$KC" set-password --config "$CFG" -r %[3]s --userid "$ID" --new-password %[2]s --temporary
"$KC" create users/"$ID"/logout --config "$CFG" -r %[3]s
`, shellQuote(email), shellQuote(password), keycloakRealm)
}
