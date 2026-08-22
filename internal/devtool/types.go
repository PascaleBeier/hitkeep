package devtool

import (
	"errors"
	"time"
)

const SchemaVersion = "hk.dev/v3"

type Variant struct {
	ID                  string            `json:"id"`
	Description         string            `json:"description"`
	BuildTags           []string          `json:"build_tags"`
	Environment         map[string]string `json:"environment,omitempty"`
	LocalImage          string            `json:"local_image"`
	Publishable         bool              `json:"publishable"`
	ProductionImageOnly bool              `json:"production_image_only"`
}

type Gate struct {
	ID              string   `json:"id"`
	Description     string   `json:"description"`
	CIGroup         string   `json:"ci_group,omitempty"`
	Command         []string `json:"command"`
	AgentCommand    []string `json:"agent_command,omitempty"`
	WorkingDir      string   `json:"working_dir,omitempty"`
	Profiles        []string `json:"profiles"`
	Paths           []string `json:"watched_inputs,omitempty"`
	ChangeAreas     []string `json:"change_areas,omitempty"`
	Depth           string   `json:"depth,omitempty"`
	Dependencies    []string `json:"dependencies,omitempty"`
	ContractVersion string   `json:"contract_version,omitempty"`
	Volatility      string   `json:"volatility,omitempty"`
	ReuseTTL        string   `json:"reuse_ttl,omitempty"`
	Weight          int      `json:"weight"`
	Timeout         string   `json:"timeout"`
}

type QAPlan struct {
	PlanID                string   `json:"plan_id"`
	Profile               string   `json:"profile"`
	BaseRef               string   `json:"base_ref,omitempty"`
	SourceSnapshot        string   `json:"source_snapshot"`
	PlannerVersion        string   `json:"planner_version"`
	CatalogVersion        string   `json:"catalog_version"`
	ChangedPaths          []string `json:"changed_paths,omitempty"`
	ChangedPathCount      int      `json:"changed_path_count,omitempty"`
	ChangedPathsTruncated bool     `json:"changed_paths_truncated,omitempty"`
	GateIDs               []string `json:"selected_gates"`
	SkippedGateIDs        []string `json:"skipped_gates,omitempty"`
	DecisionRequired      bool     `json:"decision_required"`
	DecisionReason        string   `json:"decision_reason,omitempty"`
	Escalated             bool     `json:"escalated"`
	EscalationWhy         string   `json:"escalation_reason,omitempty"`
}

type Ports struct {
	Backend  int `json:"backend"`
	Frontend int `json:"frontend"`
	SMTP     int `json:"smtp"`
	MailUI   int `json:"mail_ui"`
	E2E      int `json:"e2e"`
}

type Workspace struct {
	ID                    string       `json:"id"`
	Root                  string       `json:"root"`
	GitCommonDir          string       `json:"git_common_dir"`
	Branch                string       `json:"branch,omitempty"`
	Head                  string       `json:"head,omitempty"`
	DirtyCount            int          `json:"dirty_count"`
	ChangedPaths          []string     `json:"changed_paths,omitempty"`
	ChangedPathsTruncated bool         `json:"changed_paths_truncated,omitempty"`
	ComposeProject        string       `json:"compose_project"`
	Ports                 Ports        `json:"ports"`
	URLs                  URLs         `json:"urls"`
	StateDir              string       `json:"state_dir"`
	Services              []Service    `json:"services,omitempty"`
	ActiveRuns            []RunSummary `json:"active_runs,omitempty"`
	Dev                   *DevStatus   `json:"dev,omitempty"`
	UpdatedAt             time.Time    `json:"updated_at"`
}

type Service struct {
	Name      string `json:"name"`
	Address   string `json:"address"`
	Reachable bool   `json:"reachable"`
}

type URLs struct {
	API     string `json:"api"`
	Web     string `json:"web"`
	Mailpit string `json:"mailpit"`
}

type DevState string

const (
	DevStateStarting DevState = "starting"
	DevStateReady    DevState = "ready"
	DevStateDegraded DevState = "degraded"
	DevStateStopping DevState = "stopping"
	DevStateStopped  DevState = "stopped"
	DevStateFailed   DevState = "failed"
)

type DevOwner string

const (
	DevOwnerForeground DevOwner = "foreground"
	DevOwnerDetached   DevOwner = "detached"
)

type DevRequest struct {
	Variant string `json:"variant,omitempty"`
	Seed    bool   `json:"seed,omitempty"`
}

type DevService struct {
	Name      string    `json:"name"`
	Address   string    `json:"address"`
	Reachable bool      `json:"reachable"`
	CheckedAt time.Time `json:"checked_at"`
}

type DevStatus struct {
	State           DevState     `json:"state"`
	GenerationID    string       `json:"generation_id,omitempty"`
	Variant         string       `json:"variant,omitempty"`
	Owner           DevOwner     `json:"owner,omitempty"`
	StartedAt       *time.Time   `json:"started_at,omitempty"`
	ReadyAt         *time.Time   `json:"ready_at,omitempty"`
	StoppingAt      *time.Time   `json:"stopping_at,omitempty"`
	StoppedAt       *time.Time   `json:"stopped_at,omitempty"`
	UpdatedAt       time.Time    `json:"updated_at"`
	URLs            URLs         `json:"urls"`
	Services        []DevService `json:"services,omitempty"`
	NextEventCursor int64        `json:"next_event_cursor"`
	Error           string       `json:"error,omitempty"`
}

type DevEvent struct {
	Cursor    int64     `json:"cursor"`
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Component string    `json:"component,omitempty"`
	Level     string    `json:"level,omitempty"`
	Phase     string    `json:"phase,omitempty"`
	Message   string    `json:"message"`
}

type DevStartResult struct {
	Status       DevStatus  `json:"status"`
	Reused       bool       `json:"reused"`
	RecentEvents []DevEvent `json:"recent_events,omitempty"`
	NextCursor   int64      `json:"next_cursor"`
}

type DevLogBatch struct {
	Status            DevStatus  `json:"status"`
	Events            []DevEvent `json:"events"`
	NextCursor        int64      `json:"next_cursor"`
	EarliestCursor    int64      `json:"earliest_cursor"`
	DroppedEventCount int64      `json:"dropped_event_count"`
	Truncated         bool       `json:"truncated"`
	Complete          bool       `json:"complete"`
}

type Handoff struct {
	Workspace   Workspace    `json:"workspace"`
	RecentRuns  []RunSummary `json:"recent_runs,omitempty"`
	NextActions []string     `json:"next_actions"`
	Truncated   bool         `json:"changed_paths_truncated,omitempty"`
	GeneratedAt time.Time    `json:"generated_at"`
}

type Check struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Detected    string `json:"detected,omitempty"`
	Required    string `json:"required,omitempty"`
	Remediation string `json:"remediation,omitempty"`
}

type DoctorReport struct {
	Ready        bool               `json:"ready"`
	Capabilities DoctorCapabilities `json:"capabilities"`
	Checks       []Check            `json:"checks"`
}

type DoctorCapabilities struct {
	ContainerDevelopment bool `json:"container_development"`
	PRQA                 bool `json:"pr_qa"`
	FullQA               bool `json:"full_qa"`
}

type RunRequest struct {
	Kind    string   `json:"kind"`
	Variant string   `json:"variant,omitempty"`
	Profile string   `json:"profile,omitempty"`
	PlanID  string   `json:"plan_id,omitempty"`
	Target  string   `json:"target,omitempty"`
	GateIDs []string `json:"gate_ids,omitempty"`
}

type Run struct {
	ID          string       `json:"id"`
	WorkspaceID string       `json:"workspace_id"`
	Request     RunRequest   `json:"request"`
	Status      string       `json:"status"`
	PID         int          `json:"pid,omitempty"`
	ExitCode    *int         `json:"exit_code,omitempty"`
	LogPath     string       `json:"log_path"`
	Artifacts   []string     `json:"artifacts,omitempty"`
	GateResults []GateResult `json:"gate_results,omitempty"`
	Error       string       `json:"error,omitempty"`
	StartedAt   time.Time    `json:"started_at"`
	FinishedAt  *time.Time   `json:"finished_at,omitempty"`
}

type RunSummary struct {
	ID            string     `json:"id"`
	Request       RunRequest `json:"request"`
	Status        string     `json:"status"`
	FailedGateIDs []string   `json:"failed_gate_ids,omitempty"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	DurationMS    int64      `json:"duration_ms,omitempty"`
}

type GateResult struct {
	GateID              string     `json:"gate_id"`
	Status              string     `json:"status"`
	Error               string     `json:"error,omitempty"`
	LogPath             string     `json:"log_path"`
	StartedAt           *time.Time `json:"started_at,omitempty"`
	FinishedAt          *time.Time `json:"finished_at,omitempty"`
	DurationMS          int64      `json:"duration_ms,omitempty"`
	OriginatingRunID    string     `json:"originating_run_id,omitempty"`
	EvidenceFingerprint string     `json:"evidence_fingerprint,omitempty"`
}

type RunStart struct {
	RunID     string `json:"run_id"`
	Status    string `json:"status"`
	StatusURI string `json:"status_uri"`
	LogURI    string `json:"log_uri"`
	Reused    bool   `json:"reused,omitempty"`
}

type LogTail struct {
	RunID      string   `json:"run_id"`
	Lines      []string `json:"lines"`
	LineCount  int      `json:"line_count"`
	Truncated  bool     `json:"truncated"`
	Complete   bool     `json:"complete"`
	SourcePath string   `json:"source_path"`
	NextCursor int      `json:"next_cursor"`
}

type SourceChangeResult struct {
	Tool             string   `json:"tool"`
	Mode             string   `json:"mode"`
	Current          bool     `json:"current"`
	ChangedFiles     []string `json:"changed_files,omitempty"`
	ChangedFileCount int      `json:"changed_file_count"`
	Truncated        bool     `json:"changed_files_truncated,omitempty"`
}

type errorDataCarrier interface {
	error
	ErrorData() any
}

type CacheEntry struct {
	Kind       string    `json:"kind"`
	Key        string    `json:"key"`
	Path       string    `json:"path"`
	Bytes      int64     `json:"bytes"`
	LastUsedAt time.Time `json:"last_used_at"`
	InUse      bool      `json:"in_use"`
	Prunable   bool      `json:"prunable"`
}

type CacheReport struct {
	Root       string       `json:"root"`
	TotalBytes int64        `json:"total_bytes"`
	Entries    []CacheEntry `json:"entries"`
}

type CachePruneResult struct {
	DryRun         bool         `json:"dry_run"`
	OlderThan      string       `json:"older_than"`
	CandidateBytes int64        `json:"candidate_bytes"`
	RemovedBytes   int64        `json:"removed_bytes"`
	Candidates     []CacheEntry `json:"candidates"`
	Removed        []CacheEntry `json:"removed,omitempty"`
}

type RaceShardResult struct {
	Shard        string   `json:"shard"`
	Packages     []string `json:"packages"`
	PackageCount int      `json:"package_count"`
}

type Catalog struct {
	SchemaVersion string    `json:"schema_version"`
	Variants      []Variant `json:"variants"`
	Gates         []Gate    `json:"gates"`
	Profiles      []string  `json:"profiles"`
}

type Envelope struct {
	SchemaVersion string    `json:"schema_version"`
	Command       string    `json:"command"`
	Status        string    `json:"status"`
	WorkspaceID   string    `json:"workspace_id,omitempty"`
	Data          any       `json:"data,omitempty"`
	Error         string    `json:"error,omitempty"`
	NextActions   []string  `json:"next_actions,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

func SuccessEnvelope(command, workspaceID string, data any) Envelope {
	return Envelope{
		SchemaVersion: SchemaVersion,
		Command:       command,
		Status:        "ok",
		WorkspaceID:   workspaceID,
		Data:          data,
		Timestamp:     time.Now().UTC(),
	}
}

func ErrorEnvelope(command, workspaceID string, err error) Envelope {
	envelope := Envelope{
		SchemaVersion: SchemaVersion,
		Command:       command,
		Status:        "error",
		WorkspaceID:   workspaceID,
		Error:         redactError(err.Error()),
		Timestamp:     time.Now().UTC(),
	}
	if dataCarrier, ok := errors.AsType[errorDataCarrier](err); ok {
		envelope.Data = dataCarrier.ErrorData()
	}
	return envelope
}

type dataError struct {
	cause error
	data  any
}

func (e dataError) Error() string  { return e.cause.Error() }
func (e dataError) Unwrap() error  { return e.cause }
func (e dataError) ErrorData() any { return e.data }

func WithErrorData(err error, data any) error {
	if err == nil {
		return nil
	}
	return dataError{cause: err, data: data}
}
