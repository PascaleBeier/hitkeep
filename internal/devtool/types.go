package devtool

import (
	"errors"
	"time"
)

const SchemaVersion = "hk.dev/v1"

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
	ID          string   `json:"id"`
	Description string   `json:"description"`
	CIGroup     string   `json:"ci_group,omitempty"`
	Command     []string `json:"command"`
	WorkingDir  string   `json:"working_dir,omitempty"`
	Profiles    []string `json:"profiles"`
	Paths       []string `json:"paths,omitempty"`
	Weight      int      `json:"weight"`
	Timeout     string   `json:"timeout"`
}

type QAPlan struct {
	Profile               string   `json:"profile"`
	BaseRef               string   `json:"base_ref,omitempty"`
	ChangedPaths          []string `json:"changed_paths,omitempty"`
	ChangedPathCount      int      `json:"changed_path_count,omitempty"`
	ChangedPathsTruncated bool     `json:"changed_paths_truncated,omitempty"`
	GateIDs               []string `json:"gate_ids"`
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
	NativeDevelopment    bool `json:"native_development"`
	ContainerDevelopment bool `json:"container_development"`
	PRQA                 bool `json:"pr_qa"`
	FullQA               bool `json:"full_qa"`
}

type RunRequest struct {
	Kind    string   `json:"kind"`
	Variant string   `json:"variant,omitempty"`
	Runtime string   `json:"runtime,omitempty"`
	Seed    bool     `json:"seed,omitempty"`
	Profile string   `json:"profile,omitempty"`
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
	GateID     string     `json:"gate_id"`
	Status     string     `json:"status"`
	Error      string     `json:"error,omitempty"`
	LogPath    string     `json:"log_path"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	DurationMS int64      `json:"duration_ms,omitempty"`
}

type RunStart struct {
	RunID     string `json:"run_id"`
	Status    string `json:"status"`
	StatusURI string `json:"status_uri"`
	LogURI    string `json:"log_uri"`
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
	var dataCarrier interface{ ErrorData() any }
	if errors.As(err, &dataCarrier) {
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
