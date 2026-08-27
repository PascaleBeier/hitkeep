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

	restart := string(raw)
	start := strings.Index(restart, "restart_stateful_pod() {")
	if start < 0 {
		t.Fatal("restart_stateful_pod is missing")
	}
	restart = restart[start:]
	stop := strings.Index(restart, "stop_port_forward")
	deletePod := strings.Index(restart, "kubectl -n \"$namespace\" delete pod \"$pod\" --wait=true")
	checkPVC := strings.Index(restart, "Expected recreated pod %s to use PVC %s, got %s")
	ready := strings.Index(restart, "await_healthy")
	if stop < 0 || deletePod < 0 || checkPVC < 0 || ready < 0 || !(stop < deletePod && deletePod < checkPVC && checkPVC < ready) {
		t.Fatal("restart_stateful_pod must replace the port-forward after pod recreation and PVC verification")
	}
	if !strings.Contains(restart, "restart_stateful_pod\nfixture --verify") {
		t.Fatal("fixture verification must follow the port-forward-refreshing restart_stateful_pod call")
	}
}
