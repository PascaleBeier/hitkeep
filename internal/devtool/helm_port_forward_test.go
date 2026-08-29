package devtool

import (
	"os"
	"strings"
	"testing"
)

func TestHelmSmokeRefreshesPortForwardAfterPodRecreation(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/helm-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}

	script := string(raw)
	const declaration = "restart_stateful_pod() {\n"
	start := strings.Index(script, declaration)
	if start < 0 {
		t.Fatal("restart_stateful_pod is missing")
	}
	start += len(declaration)
	end := strings.Index(script[start:], "\n}\n\n")
	if end < 0 {
		t.Fatal("restart_stateful_pod body is unterminated")
	}
	body := script[start : start+end]

	stop := strings.Index(body, "stop_port_forward")
	deletePod := strings.Index(body, "kubectl -n \"$namespace\" delete pod \"$pod\" --wait=true")
	checkPVC := strings.Index(body, "Expected recreated pod %s to use PVC %s, got %s")
	ready := strings.Index(body, "await_healthy")
	if stop < 0 || deletePod < 0 || checkPVC < 0 || ready < 0 || !(stop < deletePod && deletePod < checkPVC && checkPVC < ready) {
		t.Fatal("restart_stateful_pod must replace the port-forward after pod recreation and PVC verification")
	}

	const restartAndVerify = "restart_stateful_pod\nfixture --verify --manifest \"$fixture_manifest\" --previous-image \"$previous_image\" --platform \"$platform\" --url \"$service_url\""
	if got := strings.Count(script, restartAndVerify); got != 3 {
		t.Fatalf("restart followed by authenticated fixture verification occurrences = %d, want 3", got)
	}
}

func TestHelmSmokeForwardsNamedHTTPServicePort(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/helm-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}

	script := string(raw)
	const namedPortForward = "kubectl -n \"$namespace\" port-forward --address 127.0.0.1 service/\"$release\" 0:http"
	if !strings.Contains(script, namedPortForward) {
		t.Fatalf("helm smoke must forward the release Service's named http port: %q", namedPortForward)
	}
	const numericPortForward = "kubectl -n \"$namespace\" port-forward --address 127.0.0.1 service/\"$release\" 0:8080"
	if strings.Contains(script, numericPortForward) {
		t.Fatalf("helm smoke must not forward hardcoded Service port 8080: %q", numericPortForward)
	}
}
