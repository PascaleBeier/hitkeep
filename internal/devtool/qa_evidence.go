package devtool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

type gateEvidence struct {
	SchemaVersion string    `json:"schema_version"`
	Fingerprint   string    `json:"fingerprint"`
	GateID        string    `json:"gate_id"`
	RunID         string    `json:"run_id"`
	CompletedAt   time.Time `json:"completed_at"`
	ExpiresAt     time.Time `json:"expires_at"`
	Runtime       string    `json:"runtime"`
}

func gateEvidenceFingerprint(gate Gate, request RunRequest) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "%s\n%s\n%s\n%s\n%s\n", gate.ID, gate.ContractVersion, request.PlanID, request.Variant, runtime.GOOS+"/"+runtime.GOARCH)
	for _, value := range gate.Command {
		fmt.Fprintln(hash, value)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (a *App) reusableGateEvidence(gate Gate, request RunRequest) (gateEvidence, bool) {
	if request.Profile == "full" || gate.ReuseTTL == "" || gate.Volatility == "release" || gate.Volatility == "live" {
		return gateEvidence{}, false
	}
	fingerprint := gateEvidenceFingerprint(gate, request)
	raw, err := os.ReadFile(filepath.Join(a.workspace.StateDir, "qa-evidence", fingerprint+".json"))
	if err != nil {
		return gateEvidence{}, false
	}
	var evidence gateEvidence
	if json.Unmarshal(raw, &evidence) != nil || evidence.SchemaVersion != SchemaVersion || evidence.Fingerprint != fingerprint || time.Now().After(evidence.ExpiresAt) {
		return gateEvidence{}, false
	}
	if gate.Volatility == "integration" && evidence.Runtime != runtime.GOOS+"/"+runtime.GOARCH {
		return gateEvidence{}, false
	}
	return evidence, true
}

func (a *App) storeGateEvidence(gate Gate, request RunRequest, runID string, completedAt time.Time) error {
	if request.Profile == "full" || gate.ReuseTTL == "" || gate.Volatility == "release" || gate.Volatility == "live" {
		return nil
	}
	ttl, err := time.ParseDuration(gate.ReuseTTL)
	if err != nil {
		return err
	}
	fingerprint := gateEvidenceFingerprint(gate, request)
	evidence := gateEvidence{SchemaVersion: SchemaVersion, Fingerprint: fingerprint, GateID: gate.ID, RunID: runID, CompletedAt: completedAt, ExpiresAt: completedAt.Add(ttl), Runtime: runtime.GOOS + "/" + runtime.GOARCH}
	path := filepath.Join(a.workspace.StateDir, "qa-evidence", fingerprint+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := writeJSONAtomic(path, evidence); err != nil {
		return err
	}
	return nil
}
