package models

import "time"

type ManagedRuntimeSource struct {
	RuntimeProvider string `json:"runtime_provider"`
	RuntimeProduct  string `json:"runtime_product"`
	BetaHeader      string `json:"beta_header,omitempty"`
	State           string `json:"state,omitempty"`
}

type ManagedAgent struct {
	ID              string               `json:"id"`
	Name            string               `json:"name"`
	Description     string               `json:"description,omitempty"`
	Model           string               `json:"model,omitempty"`
	SystemPrompt    string               `json:"system_prompt,omitempty"`
	RuntimeProvider string               `json:"runtime_provider"`
	RuntimeProduct  string               `json:"runtime_product"`
	Status          string               `json:"status"`
	ToolTypes       []string             `json:"tool_types,omitempty"`
	MCPServers      []string             `json:"mcp_servers,omitempty"`
	Skills          []string             `json:"skills,omitempty"`
	EnvironmentIDs  []string             `json:"environment_ids,omitempty"`
	Metadata        map[string]string    `json:"metadata,omitempty"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
	LastSessionAt   *time.Time           `json:"last_session_at,omitempty"`
	Source          ManagedRuntimeSource `json:"source"`
}

type ManagedEnvironment struct {
	ID                string               `json:"id"`
	Name              string               `json:"name"`
	Description       string               `json:"description,omitempty"`
	Runtime           string               `json:"runtime,omitempty"`
	ContainerTemplate string               `json:"container_template,omitempty"`
	NetworkAccess     string               `json:"network_access,omitempty"`
	MountedFiles      []string             `json:"mounted_files,omitempty"`
	Packages          []string             `json:"packages,omitempty"`
	Status            string               `json:"status"`
	Metadata          map[string]string    `json:"metadata,omitempty"`
	CreatedAt         time.Time            `json:"created_at"`
	UpdatedAt         time.Time            `json:"updated_at"`
	Source            ManagedRuntimeSource `json:"source"`
}

type ManagedSession struct {
	ID                   string               `json:"id"`
	AgentID              string               `json:"agent_id"`
	EnvironmentID        string               `json:"environment_id,omitempty"`
	Status               string               `json:"status"`
	CurrentTaskID        string               `json:"current_task_id,omitempty"`
	RuntimeProvider      string               `json:"runtime_provider"`
	RuntimeProduct       string               `json:"runtime_product"`
	PersistentFilesystem bool                 `json:"persistent_filesystem"`
	ConversationTurns    int                  `json:"conversation_turns"`
	EventCount           int                  `json:"event_count"`
	Metadata             map[string]string    `json:"metadata,omitempty"`
	StartedAt            time.Time            `json:"started_at"`
	UpdatedAt            time.Time            `json:"updated_at"`
	LastEventAt          *time.Time           `json:"last_event_at,omitempty"`
	Source               ManagedRuntimeSource `json:"source"`
}

type ManagedSessionEvent struct {
	ID        string         `json:"id"`
	SessionID string         `json:"session_id"`
	Type      string         `json:"type"`
	Role      string         `json:"role,omitempty"`
	Status    string         `json:"status,omitempty"`
	Sequence  int64          `json:"sequence"`
	Summary   string         `json:"summary,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

type ManagedTask struct {
	ID                 string            `json:"id"`
	SessionID          string            `json:"session_id"`
	ParentTaskID       string            `json:"parent_task_id,omitempty"`
	Status             string            `json:"status"`
	InputSummary       string            `json:"input_summary,omitempty"`
	OutputSummary      string            `json:"output_summary,omitempty"`
	InterruptionReason string            `json:"interruption_reason,omitempty"`
	ServerToolCount    int               `json:"server_tool_count"`
	ClientToolCount    int               `json:"client_tool_count"`
	TotalCostUSD       float64           `json:"total_cost_usd"`
	TotalTokens        int64             `json:"total_tokens"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	StartedAt          time.Time         `json:"started_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
	CompletedAt        *time.Time        `json:"completed_at,omitempty"`
}

type ManagedArtifact struct {
	ID          string            `json:"id"`
	TaskID      string            `json:"task_id"`
	SessionID   string            `json:"session_id,omitempty"`
	Name        string            `json:"name"`
	Kind        string            `json:"kind"`
	URI         string            `json:"uri,omitempty"`
	ContentType string            `json:"content_type,omitempty"`
	Status      string            `json:"status"`
	SizeBytes   int64             `json:"size_bytes,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

type ManagedTaskDecisionRequest struct {
	Reason string `json:"reason,omitempty"`
}

type ManagedAgentUpsertRequest struct {
	ID              string            `json:"id,omitempty"`
	Name            string            `json:"name"`
	Description     string            `json:"description,omitempty"`
	Model           string            `json:"model,omitempty"`
	SystemPrompt    string            `json:"system_prompt,omitempty"`
	RuntimeProvider string            `json:"runtime_provider,omitempty"`
	RuntimeProduct  string            `json:"runtime_product,omitempty"`
	Status          string            `json:"status,omitempty"`
	ToolTypes       []string          `json:"tool_types,omitempty"`
	MCPServers      []string          `json:"mcp_servers,omitempty"`
	Skills          []string          `json:"skills,omitempty"`
	EnvironmentIDs  []string          `json:"environment_ids,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	LastSessionAt   *time.Time        `json:"last_session_at,omitempty"`
}

type ManagedEnvironmentUpsertRequest struct {
	ID                string            `json:"id,omitempty"`
	Name              string            `json:"name"`
	Description       string            `json:"description,omitempty"`
	Runtime           string            `json:"runtime,omitempty"`
	ContainerTemplate string            `json:"container_template,omitempty"`
	NetworkAccess     string            `json:"network_access,omitempty"`
	MountedFiles      []string          `json:"mounted_files,omitempty"`
	Packages          []string          `json:"packages,omitempty"`
	Status            string            `json:"status,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

type ManagedSessionCreateRequest struct {
	ID                   string            `json:"id,omitempty"`
	AgentID              string            `json:"agent_id"`
	EnvironmentID        string            `json:"environment_id,omitempty"`
	Status               string            `json:"status,omitempty"`
	CurrentTaskID        string            `json:"current_task_id,omitempty"`
	RuntimeProvider      string            `json:"runtime_provider,omitempty"`
	RuntimeProduct       string            `json:"runtime_product,omitempty"`
	PersistentFilesystem *bool             `json:"persistent_filesystem,omitempty"`
	ConversationTurns    int               `json:"conversation_turns,omitempty"`
	EventCount           int               `json:"event_count,omitempty"`
	Metadata             map[string]string `json:"metadata,omitempty"`
	StartedAt            *time.Time        `json:"started_at,omitempty"`
}

type ManagedSessionEventCreateRequest struct {
	ID        string         `json:"id,omitempty"`
	Type      string         `json:"type"`
	Role      string         `json:"role,omitempty"`
	Status    string         `json:"status,omitempty"`
	Sequence  int64          `json:"sequence,omitempty"`
	Summary   string         `json:"summary,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	CreatedAt *time.Time     `json:"created_at,omitempty"`
}

type ManagedTaskUpsertRequest struct {
	ID                 string            `json:"id,omitempty"`
	SessionID          string            `json:"session_id"`
	ParentTaskID       string            `json:"parent_task_id,omitempty"`
	Status             string            `json:"status,omitempty"`
	InputSummary       string            `json:"input_summary,omitempty"`
	OutputSummary      string            `json:"output_summary,omitempty"`
	InterruptionReason string            `json:"interruption_reason,omitempty"`
	ServerToolCount    int               `json:"server_tool_count,omitempty"`
	ClientToolCount    int               `json:"client_tool_count,omitempty"`
	TotalCostUSD       float64           `json:"total_cost_usd,omitempty"`
	TotalTokens        int64             `json:"total_tokens,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	StartedAt          *time.Time        `json:"started_at,omitempty"`
	CompletedAt        *time.Time        `json:"completed_at,omitempty"`
}

type ManagedArtifactCreateRequest struct {
	ID          string            `json:"id,omitempty"`
	SessionID   string            `json:"session_id,omitempty"`
	Name        string            `json:"name"`
	Kind        string            `json:"kind"`
	URI         string            `json:"uri,omitempty"`
	ContentType string            `json:"content_type,omitempty"`
	Status      string            `json:"status,omitempty"`
	SizeBytes   int64             `json:"size_bytes,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}
