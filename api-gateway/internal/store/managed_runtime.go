package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/govagn/api-gateway/internal/models"
)

func encodeJSONStringSlice(values []string) string {
	if values == nil {
		return "[]"
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			normalized = append(normalized, trimmed)
		}
	}
	raw, _ := json.Marshal(normalized)
	return string(raw)
}

func decodeJSONStringSlice(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

func encodeJSONStringMap(values map[string]string) string {
	if values == nil {
		return "{}"
	}
	raw, _ := json.Marshal(values)
	return string(raw)
}

func decodeJSONStringMap(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return map[string]string{}
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return map[string]string{}
	}
	return out
}

func decodeJSONAnyMap(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func runtimeSourceFromColumns(provider, product string) models.ManagedRuntimeSource {
	return models.ManagedRuntimeSource{
		RuntimeProvider: strings.TrimSpace(provider),
		RuntimeProduct:  strings.TrimSpace(product),
		BetaHeader:      "managed-agents-2026-04-01",
		State:           "persisted",
	}
}

func nullTimeOrNow(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

func nullTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}

func (s *PostgresStore) UpsertManagedAgent(ctx context.Context, tenantID string, agent models.ManagedAgent) (models.ManagedAgent, error) {
	agent.ID = strings.TrimSpace(agent.ID)
	if agent.ID == "" {
		agent.ID = generateStoreID()
	}
	agent.Name = strings.TrimSpace(agent.Name)
	if agent.Name == "" {
		return agent, fmt.Errorf("agent name is required")
	}
	agent.RuntimeProvider = strings.TrimSpace(agent.RuntimeProvider)
	if agent.RuntimeProvider == "" {
		agent.RuntimeProvider = "anthropic"
	}
	agent.RuntimeProduct = strings.TrimSpace(agent.RuntimeProduct)
	if agent.RuntimeProduct == "" {
		agent.RuntimeProduct = "claude_managed_agents"
	}
	agent.Status = strings.TrimSpace(agent.Status)
	if agent.Status == "" {
		agent.Status = "active"
	}

	var lastSessionAt any
	if agent.LastSessionAt != nil {
		lastSessionAt = agent.LastSessionAt.UTC()
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO managed_agents (
			tenant_id, agent_id, name, description, model, system_prompt,
			runtime_provider, runtime_product, status, tool_types, mcp_servers,
			skills, environment_ids, metadata, last_session_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11::jsonb, $12::jsonb, $13::jsonb, $14::jsonb, $15)
		ON CONFLICT (tenant_id, agent_id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			model = EXCLUDED.model,
			system_prompt = EXCLUDED.system_prompt,
			runtime_provider = EXCLUDED.runtime_provider,
			runtime_product = EXCLUDED.runtime_product,
			status = EXCLUDED.status,
			tool_types = EXCLUDED.tool_types,
			mcp_servers = EXCLUDED.mcp_servers,
			skills = EXCLUDED.skills,
			environment_ids = EXCLUDED.environment_ids,
			metadata = EXCLUDED.metadata,
			last_session_at = COALESCE(EXCLUDED.last_session_at, managed_agents.last_session_at),
			updated_at = NOW()
		RETURNING created_at, updated_at, last_session_at
	`,
		tenantID,
		agent.ID,
		agent.Name,
		strings.TrimSpace(agent.Description),
		strings.TrimSpace(agent.Model),
		strings.TrimSpace(agent.SystemPrompt),
		agent.RuntimeProvider,
		agent.RuntimeProduct,
		agent.Status,
		encodeJSONStringSlice(agent.ToolTypes),
		encodeJSONStringSlice(agent.MCPServers),
		encodeJSONStringSlice(agent.Skills),
		encodeJSONStringSlice(agent.EnvironmentIDs),
		encodeJSONStringMap(agent.Metadata),
		lastSessionAt,
	).Scan(&agent.CreatedAt, &agent.UpdatedAt, &agent.LastSessionAt)
	if err != nil {
		return agent, err
	}
	agent.Source = runtimeSourceFromColumns(agent.RuntimeProvider, agent.RuntimeProduct)
	return agent, nil
}

func (s *PostgresStore) ListManagedAgents(ctx context.Context, tenantID string, limit int) (*models.Page[models.ManagedAgent], error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT agent_id, name, description, model, system_prompt, runtime_provider, runtime_product,
		       status, tool_types::text, mcp_servers::text, skills::text, environment_ids::text,
		       metadata::text, created_at, updated_at, last_session_at
		FROM managed_agents
		WHERE tenant_id = $1
		ORDER BY updated_at DESC, agent_id DESC
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]models.ManagedAgent, 0, limit)
	for rows.Next() {
		var item models.ManagedAgent
		var toolTypesJSON, mcpServersJSON, skillsJSON, envIDsJSON, metadataJSON string
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Description, &item.Model, &item.SystemPrompt,
			&item.RuntimeProvider, &item.RuntimeProduct, &item.Status,
			&toolTypesJSON, &mcpServersJSON, &skillsJSON, &envIDsJSON, &metadataJSON,
			&item.CreatedAt, &item.UpdatedAt, &item.LastSessionAt,
		); err != nil {
			return nil, err
		}
		item.ToolTypes = decodeJSONStringSlice(toolTypesJSON)
		item.MCPServers = decodeJSONStringSlice(mcpServersJSON)
		item.Skills = decodeJSONStringSlice(skillsJSON)
		item.EnvironmentIDs = decodeJSONStringSlice(envIDsJSON)
		item.Metadata = decodeJSONStringMap(metadataJSON)
		item.Source = runtimeSourceFromColumns(item.RuntimeProvider, item.RuntimeProduct)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &models.Page[models.ManagedAgent]{Items: items, Total: int64(len(items)), HasMore: false}, nil
}

func (s *PostgresStore) GetManagedAgent(ctx context.Context, tenantID, agentID string) (models.ManagedAgent, error) {
	var item models.ManagedAgent
	var toolTypesJSON, mcpServersJSON, skillsJSON, envIDsJSON, metadataJSON string
	err := s.pool.QueryRow(ctx, `
		SELECT agent_id, name, description, model, system_prompt, runtime_provider, runtime_product,
		       status, tool_types::text, mcp_servers::text, skills::text, environment_ids::text,
		       metadata::text, created_at, updated_at, last_session_at
		FROM managed_agents
		WHERE tenant_id = $1 AND agent_id = $2
	`, tenantID, agentID).Scan(
		&item.ID, &item.Name, &item.Description, &item.Model, &item.SystemPrompt,
		&item.RuntimeProvider, &item.RuntimeProduct, &item.Status,
		&toolTypesJSON, &mcpServersJSON, &skillsJSON, &envIDsJSON, &metadataJSON,
		&item.CreatedAt, &item.UpdatedAt, &item.LastSessionAt,
	)
	if err != nil {
		return item, err
	}
	item.ToolTypes = decodeJSONStringSlice(toolTypesJSON)
	item.MCPServers = decodeJSONStringSlice(mcpServersJSON)
	item.Skills = decodeJSONStringSlice(skillsJSON)
	item.EnvironmentIDs = decodeJSONStringSlice(envIDsJSON)
	item.Metadata = decodeJSONStringMap(metadataJSON)
	item.Source = runtimeSourceFromColumns(item.RuntimeProvider, item.RuntimeProduct)
	return item, nil
}

func (s *PostgresStore) UpsertManagedEnvironment(ctx context.Context, tenantID string, environment models.ManagedEnvironment) (models.ManagedEnvironment, error) {
	environment.ID = strings.TrimSpace(environment.ID)
	if environment.ID == "" {
		environment.ID = generateStoreID()
	}
	environment.Name = strings.TrimSpace(environment.Name)
	if environment.Name == "" {
		return environment, fmt.Errorf("environment name is required")
	}
	environment.Status = strings.TrimSpace(environment.Status)
	if environment.Status == "" {
		environment.Status = "active"
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO managed_environments (
			tenant_id, environment_id, name, description, runtime, container_template,
			network_access, mounted_files, packages, status, metadata
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9::jsonb, $10, $11::jsonb)
		ON CONFLICT (tenant_id, environment_id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			runtime = EXCLUDED.runtime,
			container_template = EXCLUDED.container_template,
			network_access = EXCLUDED.network_access,
			mounted_files = EXCLUDED.mounted_files,
			packages = EXCLUDED.packages,
			status = EXCLUDED.status,
			metadata = EXCLUDED.metadata,
			updated_at = NOW()
		RETURNING created_at, updated_at
	`,
		tenantID,
		environment.ID,
		environment.Name,
		strings.TrimSpace(environment.Description),
		strings.TrimSpace(environment.Runtime),
		strings.TrimSpace(environment.ContainerTemplate),
		strings.TrimSpace(environment.NetworkAccess),
		encodeJSONStringSlice(environment.MountedFiles),
		encodeJSONStringSlice(environment.Packages),
		environment.Status,
		encodeJSONStringMap(environment.Metadata),
	).Scan(&environment.CreatedAt, &environment.UpdatedAt)
	if err != nil {
		return environment, err
	}
	environment.Source = runtimeSourceFromColumns("anthropic", "claude_managed_agents")
	return environment, nil
}

func (s *PostgresStore) ListManagedEnvironments(ctx context.Context, tenantID string, limit int) (*models.Page[models.ManagedEnvironment], error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT environment_id, name, description, runtime, container_template, network_access,
		       mounted_files::text, packages::text, status, metadata::text, created_at, updated_at
		FROM managed_environments
		WHERE tenant_id = $1
		ORDER BY updated_at DESC, environment_id DESC
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]models.ManagedEnvironment, 0, limit)
	for rows.Next() {
		var item models.ManagedEnvironment
		var mountedJSON, packagesJSON, metadataJSON string
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Description, &item.Runtime, &item.ContainerTemplate,
			&item.NetworkAccess, &mountedJSON, &packagesJSON, &item.Status, &metadataJSON,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.MountedFiles = decodeJSONStringSlice(mountedJSON)
		item.Packages = decodeJSONStringSlice(packagesJSON)
		item.Metadata = decodeJSONStringMap(metadataJSON)
		item.Source = runtimeSourceFromColumns("anthropic", "claude_managed_agents")
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &models.Page[models.ManagedEnvironment]{Items: items, Total: int64(len(items)), HasMore: false}, nil
}

func (s *PostgresStore) GetManagedEnvironment(ctx context.Context, tenantID, environmentID string) (models.ManagedEnvironment, error) {
	var item models.ManagedEnvironment
	var mountedJSON, packagesJSON, metadataJSON string
	err := s.pool.QueryRow(ctx, `
		SELECT environment_id, name, description, runtime, container_template, network_access,
		       mounted_files::text, packages::text, status, metadata::text, created_at, updated_at
		FROM managed_environments
		WHERE tenant_id = $1 AND environment_id = $2
	`, tenantID, environmentID).Scan(
		&item.ID, &item.Name, &item.Description, &item.Runtime, &item.ContainerTemplate,
		&item.NetworkAccess, &mountedJSON, &packagesJSON, &item.Status, &metadataJSON,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return item, err
	}
	item.MountedFiles = decodeJSONStringSlice(mountedJSON)
	item.Packages = decodeJSONStringSlice(packagesJSON)
	item.Metadata = decodeJSONStringMap(metadataJSON)
	item.Source = runtimeSourceFromColumns("anthropic", "claude_managed_agents")
	return item, nil
}

func (s *PostgresStore) CreateManagedSession(ctx context.Context, tenantID string, session models.ManagedSession) (models.ManagedSession, error) {
	session.ID = strings.TrimSpace(session.ID)
	if session.ID == "" {
		session.ID = generateStoreID()
	}
	session.AgentID = strings.TrimSpace(session.AgentID)
	if session.AgentID == "" {
		return session, fmt.Errorf("agent_id is required")
	}
	session.Status = strings.TrimSpace(session.Status)
	if session.Status == "" {
		session.Status = "running"
	}
	if session.RuntimeProvider == "" {
		session.RuntimeProvider = "anthropic"
	}
	if session.RuntimeProduct == "" {
		session.RuntimeProduct = "claude_managed_agents"
	}
	if !session.PersistentFilesystem {
		session.PersistentFilesystem = true
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO managed_sessions (
			tenant_id, session_id, agent_id, environment_id, status, current_task_id,
			runtime_provider, runtime_product, persistent_filesystem, conversation_turns,
			event_count, metadata, started_at, updated_at, last_event_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13, $14, $15)
		RETURNING started_at, updated_at, last_event_at
	`,
		tenantID,
		session.ID,
		session.AgentID,
		strings.TrimSpace(session.EnvironmentID),
		session.Status,
		strings.TrimSpace(session.CurrentTaskID),
		session.RuntimeProvider,
		session.RuntimeProduct,
		session.PersistentFilesystem,
		session.ConversationTurns,
		session.EventCount,
		encodeJSONStringMap(session.Metadata),
		nullTimeOrNow(session.StartedAt),
		nullTimeOrNow(session.UpdatedAt),
		nullTime(session.LastEventAt),
	).Scan(&session.StartedAt, &session.UpdatedAt, &session.LastEventAt)
	if err != nil {
		return session, err
	}
	session.Source = runtimeSourceFromColumns(session.RuntimeProvider, session.RuntimeProduct)
	_, _ = s.pool.Exec(ctx, `
		UPDATE managed_agents
		SET last_session_at = GREATEST(COALESCE(last_session_at, $3), $3),
		    updated_at = NOW()
		WHERE tenant_id = $1 AND agent_id = $2
	`, tenantID, session.AgentID, session.StartedAt)
	return session, nil
}

func (s *PostgresStore) ListManagedSessions(ctx context.Context, tenantID, agentID string, limit int) (*models.Page[models.ManagedSession], error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	args := []any{tenantID}
	query := `
		SELECT session_id, agent_id, environment_id, status, current_task_id, runtime_provider, runtime_product,
		       persistent_filesystem, conversation_turns, event_count, metadata::text,
		       started_at, updated_at, last_event_at
		FROM managed_sessions
		WHERE tenant_id = $1`
	if trimmed := strings.TrimSpace(agentID); trimmed != "" {
		args = append(args, trimmed)
		query += fmt.Sprintf(" AND agent_id = $%d", len(args))
	}
	args = append(args, limit)
	query += fmt.Sprintf(" ORDER BY updated_at DESC, session_id DESC LIMIT $%d", len(args))
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]models.ManagedSession, 0, limit)
	for rows.Next() {
		var item models.ManagedSession
		var metadataJSON string
		if err := rows.Scan(
			&item.ID, &item.AgentID, &item.EnvironmentID, &item.Status, &item.CurrentTaskID,
			&item.RuntimeProvider, &item.RuntimeProduct, &item.PersistentFilesystem,
			&item.ConversationTurns, &item.EventCount, &metadataJSON,
			&item.StartedAt, &item.UpdatedAt, &item.LastEventAt,
		); err != nil {
			return nil, err
		}
		item.Metadata = decodeJSONStringMap(metadataJSON)
		item.Source = runtimeSourceFromColumns(item.RuntimeProvider, item.RuntimeProduct)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &models.Page[models.ManagedSession]{Items: items, Total: int64(len(items)), HasMore: false}, nil
}

func (s *PostgresStore) GetManagedSession(ctx context.Context, tenantID, sessionID string) (models.ManagedSession, error) {
	var item models.ManagedSession
	var metadataJSON string
	err := s.pool.QueryRow(ctx, `
		SELECT session_id, agent_id, environment_id, status, current_task_id, runtime_provider, runtime_product,
		       persistent_filesystem, conversation_turns, event_count, metadata::text,
		       started_at, updated_at, last_event_at
		FROM managed_sessions
		WHERE tenant_id = $1 AND session_id = $2
	`, tenantID, sessionID).Scan(
		&item.ID, &item.AgentID, &item.EnvironmentID, &item.Status, &item.CurrentTaskID,
		&item.RuntimeProvider, &item.RuntimeProduct, &item.PersistentFilesystem,
		&item.ConversationTurns, &item.EventCount, &metadataJSON,
		&item.StartedAt, &item.UpdatedAt, &item.LastEventAt,
	)
	if err != nil {
		return item, err
	}
	item.Metadata = decodeJSONStringMap(metadataJSON)
	item.Source = runtimeSourceFromColumns(item.RuntimeProvider, item.RuntimeProduct)
	return item, nil
}

func (s *PostgresStore) CreateManagedSessionEvent(ctx context.Context, tenantID string, event models.ManagedSessionEvent) (models.ManagedSessionEvent, error) {
	event.ID = strings.TrimSpace(event.ID)
	if event.ID == "" {
		event.ID = generateStoreID()
	}
	event.SessionID = strings.TrimSpace(event.SessionID)
	if event.SessionID == "" {
		return event, fmt.Errorf("session_id is required")
	}
	event.Type = strings.TrimSpace(event.Type)
	if event.Type == "" {
		return event, fmt.Errorf("event type is required")
	}
	if event.Data == nil {
		event.Data = map[string]any{}
	}
	createdAt := time.Now().UTC()
	if !event.CreatedAt.IsZero() {
		createdAt = event.CreatedAt.UTC()
	}
	role := strings.TrimSpace(event.Role)
	status := strings.TrimSpace(event.Status)
	summary := strings.TrimSpace(event.Summary)
	sequence := event.Sequence

	if err := s.pool.QueryRow(ctx, `
		WITH next_sequence AS (
			SELECT CASE
				WHEN $4::bigint > 0 THEN $4::bigint
				ELSE COALESCE(MAX(sequence_num), 0) + 1
			END AS sequence_num
			FROM managed_session_events
			WHERE tenant_id = $1 AND session_id = $2
		)
		INSERT INTO managed_session_events (
			tenant_id, event_id, session_id, event_type, role, status,
			sequence_num, summary, data, created_at
		)
		SELECT $1, $3, $2, $5, $6, $7, next_sequence.sequence_num, $8, $9::jsonb, $10
		FROM next_sequence
		ON CONFLICT (tenant_id, event_id) DO UPDATE SET
			event_type = EXCLUDED.event_type,
			role = EXCLUDED.role,
			status = EXCLUDED.status,
			sequence_num = EXCLUDED.sequence_num,
			summary = EXCLUDED.summary,
			data = EXCLUDED.data,
			created_at = EXCLUDED.created_at
		RETURNING sequence_num, created_at
	`, tenantID, event.SessionID, event.ID, sequence, event.Type, role, status, summary, mustJSON(event.Data), createdAt).Scan(&event.Sequence, &event.CreatedAt); err != nil {
		return event, err
	}
	event.Role = role
	event.Status = status
	event.Summary = summary

	turnDelta := 0
	switch role {
	case "user", "assistant", "system":
		turnDelta = 1
	}
	_, _ = s.pool.Exec(ctx, `
		UPDATE managed_sessions AS s
		SET event_count = counts.event_count,
		    conversation_turns = s.conversation_turns + $3,
		    updated_at = GREATEST(s.updated_at, $2),
		    last_event_at = $2,
		    status = CASE
		    	WHEN $4 <> '' THEN $4
		    	ELSE s.status
		    END
		FROM (
			SELECT COUNT(*)::integer AS event_count
			FROM managed_session_events
			WHERE tenant_id = $1 AND session_id = $5
		) AS counts
		WHERE s.tenant_id = $1 AND s.session_id = $5
	`, tenantID, event.CreatedAt, turnDelta, status, event.SessionID)

	return event, nil
}

func (s *PostgresStore) ListManagedSessionEvents(ctx context.Context, tenantID, sessionID string, limit int) (*models.Page[models.ManagedSessionEvent], error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.pool.Query(ctx, `
		SELECT event_id, session_id, event_type, role, status, sequence_num, summary, data::text, created_at
		FROM managed_session_events
		WHERE tenant_id = $1 AND session_id = $2
		ORDER BY sequence_num ASC, created_at ASC
		LIMIT $3
	`, tenantID, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]models.ManagedSessionEvent, 0, limit)
	for rows.Next() {
		var item models.ManagedSessionEvent
		var dataJSON string
		if err := rows.Scan(
			&item.ID, &item.SessionID, &item.Type, &item.Role, &item.Status,
			&item.Sequence, &item.Summary, &dataJSON, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		item.Data = decodeJSONAnyMap(dataJSON)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &models.Page[models.ManagedSessionEvent]{Items: items, Total: int64(len(items)), HasMore: false}, nil
}

func (s *PostgresStore) UpsertManagedTask(ctx context.Context, tenantID string, task models.ManagedTask) (models.ManagedTask, error) {
	task.ID = strings.TrimSpace(task.ID)
	if task.ID == "" {
		task.ID = generateStoreID()
	}
	task.SessionID = strings.TrimSpace(task.SessionID)
	if task.SessionID == "" {
		return task, fmt.Errorf("session_id is required")
	}
	task.Status = strings.TrimSpace(task.Status)
	if task.Status == "" {
		task.Status = "running"
	}
	startedAt := time.Now().UTC()
	if !task.StartedAt.IsZero() {
		startedAt = task.StartedAt.UTC()
	}
	var completedAt any
	if task.CompletedAt != nil {
		completedAt = task.CompletedAt.UTC()
	} else if isManagedTaskTerminal(task.Status) {
		now := time.Now().UTC()
		task.CompletedAt = &now
		completedAt = now
	}
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO managed_tasks (
			tenant_id, task_id, session_id, parent_task_id, status,
			input_summary, output_summary, interruption_reason,
			server_tool_count, client_tool_count, total_cost_usd, total_tokens,
			metadata, started_at, updated_at, completed_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::jsonb, $14, NOW(), $15)
		ON CONFLICT (tenant_id, task_id) DO UPDATE SET
			session_id = EXCLUDED.session_id,
			parent_task_id = EXCLUDED.parent_task_id,
			status = EXCLUDED.status,
			input_summary = EXCLUDED.input_summary,
			output_summary = EXCLUDED.output_summary,
			interruption_reason = EXCLUDED.interruption_reason,
			server_tool_count = EXCLUDED.server_tool_count,
			client_tool_count = EXCLUDED.client_tool_count,
			total_cost_usd = EXCLUDED.total_cost_usd,
			total_tokens = EXCLUDED.total_tokens,
			metadata = EXCLUDED.metadata,
			started_at = LEAST(managed_tasks.started_at, EXCLUDED.started_at),
			updated_at = NOW(),
			completed_at = COALESCE(EXCLUDED.completed_at, managed_tasks.completed_at)
		RETURNING started_at, updated_at, completed_at
	`, tenantID, task.ID, task.SessionID, strings.TrimSpace(task.ParentTaskID), task.Status,
		strings.TrimSpace(task.InputSummary), strings.TrimSpace(task.OutputSummary), strings.TrimSpace(task.InterruptionReason),
		task.ServerToolCount, task.ClientToolCount, task.TotalCostUSD, task.TotalTokens,
		encodeJSONStringMap(task.Metadata), startedAt, completedAt,
	).Scan(&task.StartedAt, &task.UpdatedAt, &task.CompletedAt); err != nil {
		return task, err
	}

	_, _ = s.pool.Exec(ctx, `
		UPDATE managed_sessions
		SET current_task_id = $3,
		    updated_at = NOW()
		WHERE tenant_id = $1 AND session_id = $2
	`, tenantID, task.SessionID, task.ID)
	_ = s.syncManagedTaskRun(ctx, tenantID, task)
	return task, nil
}

func (s *PostgresStore) ListManagedSessionTasks(ctx context.Context, tenantID, sessionID string, limit int) (*models.Page[models.ManagedTask], error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT task_id, session_id, parent_task_id, status, input_summary, output_summary,
		       interruption_reason, server_tool_count, client_tool_count, total_cost_usd,
		       total_tokens, metadata::text, started_at, updated_at, completed_at
		FROM managed_tasks
		WHERE tenant_id = $1 AND session_id = $2
		ORDER BY updated_at DESC, task_id DESC
		LIMIT $3
	`, tenantID, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]models.ManagedTask, 0, limit)
	for rows.Next() {
		var item models.ManagedTask
		var metadataJSON string
		if err := rows.Scan(
			&item.ID, &item.SessionID, &item.ParentTaskID, &item.Status, &item.InputSummary, &item.OutputSummary,
			&item.InterruptionReason, &item.ServerToolCount, &item.ClientToolCount, &item.TotalCostUSD,
			&item.TotalTokens, &metadataJSON, &item.StartedAt, &item.UpdatedAt, &item.CompletedAt,
		); err != nil {
			return nil, err
		}
		item.Metadata = decodeJSONStringMap(metadataJSON)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &models.Page[models.ManagedTask]{Items: items, Total: int64(len(items)), HasMore: false}, nil
}

func (s *PostgresStore) GetManagedTask(ctx context.Context, tenantID, taskID string) (models.ManagedTask, error) {
	var item models.ManagedTask
	var metadataJSON string
	err := s.pool.QueryRow(ctx, `
		SELECT task_id, session_id, parent_task_id, status, input_summary, output_summary,
		       interruption_reason, server_tool_count, client_tool_count, total_cost_usd,
		       total_tokens, metadata::text, started_at, updated_at, completed_at
		FROM managed_tasks
		WHERE tenant_id = $1 AND task_id = $2
	`, tenantID, taskID).Scan(
		&item.ID, &item.SessionID, &item.ParentTaskID, &item.Status, &item.InputSummary, &item.OutputSummary,
		&item.InterruptionReason, &item.ServerToolCount, &item.ClientToolCount, &item.TotalCostUSD,
		&item.TotalTokens, &metadataJSON, &item.StartedAt, &item.UpdatedAt, &item.CompletedAt,
	)
	if err != nil {
		return item, err
	}
	item.Metadata = decodeJSONStringMap(metadataJSON)
	return item, nil
}

func (s *PostgresStore) CreateManagedArtifact(ctx context.Context, tenantID string, artifact models.ManagedArtifact) (models.ManagedArtifact, error) {
	artifact.ID = strings.TrimSpace(artifact.ID)
	if artifact.ID == "" {
		artifact.ID = generateStoreID()
	}
	artifact.TaskID = strings.TrimSpace(artifact.TaskID)
	if artifact.TaskID == "" {
		return artifact, fmt.Errorf("task_id is required")
	}
	artifact.Name = strings.TrimSpace(artifact.Name)
	if artifact.Name == "" {
		return artifact, fmt.Errorf("artifact name is required")
	}
	artifact.Kind = strings.TrimSpace(artifact.Kind)
	if artifact.Kind == "" {
		return artifact, fmt.Errorf("artifact kind is required")
	}
	artifact.Status = strings.TrimSpace(artifact.Status)
	if artifact.Status == "" {
		artifact.Status = "ready"
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO managed_artifacts (
			tenant_id, artifact_id, task_id, session_id, name, kind, uri,
			content_type, status, size_bytes, metadata
		)
		VALUES (
			$1, $2, $3,
			COALESCE(NULLIF($4, ''), (SELECT session_id FROM managed_tasks WHERE tenant_id = $1 AND task_id = $3)),
			$5, $6, $7, $8, $9, $10, $11::jsonb
		)
		RETURNING session_id, created_at
	`, tenantID, artifact.ID, artifact.TaskID, strings.TrimSpace(artifact.SessionID), artifact.Name, artifact.Kind,
		strings.TrimSpace(artifact.URI), strings.TrimSpace(artifact.ContentType), artifact.Status, artifact.SizeBytes,
		encodeJSONStringMap(artifact.Metadata),
	).Scan(&artifact.SessionID, &artifact.CreatedAt)
	return artifact, err
}

func (s *PostgresStore) ListManagedTaskArtifacts(ctx context.Context, tenantID, taskID string, limit int) (*models.Page[models.ManagedArtifact], error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT artifact_id, task_id, session_id, name, kind, uri, content_type, status,
		       size_bytes, metadata::text, created_at
		FROM managed_artifacts
		WHERE tenant_id = $1 AND task_id = $2
		ORDER BY created_at DESC, artifact_id DESC
		LIMIT $3
	`, tenantID, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]models.ManagedArtifact, 0, limit)
	for rows.Next() {
		var item models.ManagedArtifact
		var metadataJSON string
		if err := rows.Scan(
			&item.ID, &item.TaskID, &item.SessionID, &item.Name, &item.Kind, &item.URI, &item.ContentType,
			&item.Status, &item.SizeBytes, &metadataJSON, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		item.Metadata = decodeJSONStringMap(metadataJSON)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &models.Page[models.ManagedArtifact]{Items: items, Total: int64(len(items)), HasMore: false}, nil
}

func (s *PostgresStore) ApproveManagedTask(ctx context.Context, tenantID, taskID, actor, reason string) (models.ManagedTask, error) {
	return s.decideManagedTask(ctx, tenantID, taskID, actor, reason, "approve")
}

func (s *PostgresStore) DenyManagedTask(ctx context.Context, tenantID, taskID, actor, reason string) (models.ManagedTask, error) {
	return s.decideManagedTask(ctx, tenantID, taskID, actor, reason, "deny")
}

func (s *PostgresStore) decideManagedTask(ctx context.Context, tenantID, taskID, actor, reason, action string) (models.ManagedTask, error) {
	var task models.ManagedTask
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return task, err
	}
	defer tx.Rollback(ctx)

	var metadataJSON string
	if err := tx.QueryRow(ctx, `
		SELECT task_id, session_id, parent_task_id, status, input_summary, output_summary,
		       interruption_reason, server_tool_count, client_tool_count, total_cost_usd,
		       total_tokens, metadata::text, started_at, updated_at, completed_at
		FROM managed_tasks
		WHERE tenant_id = $1 AND task_id = $2
		FOR UPDATE
	`, tenantID, taskID).Scan(
		&task.ID, &task.SessionID, &task.ParentTaskID, &task.Status, &task.InputSummary, &task.OutputSummary,
		&task.InterruptionReason, &task.ServerToolCount, &task.ClientToolCount, &task.TotalCostUSD,
		&task.TotalTokens, &metadataJSON, &task.StartedAt, &task.UpdatedAt, &task.CompletedAt,
	); err != nil {
		return task, err
	}
	task.Metadata = decodeJSONStringMap(metadataJSON)

	now := time.Now().UTC()
	nextStatus := "approved"
	completedAt := any(nil)
	if action == "deny" {
		nextStatus = "denied"
		completedAt = now
	}
	if reason = strings.TrimSpace(reason); action == "deny" && reason != "" {
		task.InterruptionReason = reason
	}
	if err := tx.QueryRow(ctx, `
		UPDATE managed_tasks
		SET status = $3,
		    interruption_reason = CASE
		    	WHEN $4 <> '' THEN $4
		    	ELSE interruption_reason
		    END,
		    updated_at = $5,
		    completed_at = COALESCE($6, completed_at)
		WHERE tenant_id = $1 AND task_id = $2
		RETURNING status, interruption_reason, updated_at, completed_at
	`, tenantID, taskID, nextStatus, reason, now, completedAt).Scan(&task.Status, &task.InterruptionReason, &task.UpdatedAt, &task.CompletedAt); err != nil {
		return task, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO managed_task_decisions (tenant_id, task_id, action, actor, reason)
		VALUES ($1, $2, $3, $4, $5)
	`, tenantID, taskID, action, strings.TrimSpace(actor), reason); err != nil {
		return task, err
	}

	var eventCreatedAt time.Time
	if err := tx.QueryRow(ctx, `
		WITH next_sequence AS (
			SELECT COALESCE(MAX(sequence_num), 0) + 1 AS sequence_num
			FROM managed_session_events
			WHERE tenant_id = $1 AND session_id = $2
		)
		INSERT INTO managed_session_events (
			tenant_id, event_id, session_id, event_type, role, status, sequence_num, summary, data, created_at
		)
		SELECT
			$1,
			$3,
			$2,
			'approval.decision',
			'system',
			$4,
			next_sequence.sequence_num,
			$5,
			$6::jsonb,
			$7
		FROM next_sequence
		RETURNING created_at
	`, tenantID, task.SessionID, generateStoreID(), nextStatus,
		fmt.Sprintf("Task %s %s", taskID, nextStatus),
		mustJSON(map[string]any{
			"task_id": taskID,
			"action":  action,
			"actor":   strings.TrimSpace(actor),
			"reason":  reason,
		}),
		now,
	).Scan(&eventCreatedAt); err != nil {
		return task, err
	}

	sessionStatus := "running"
	if action == "deny" {
		sessionStatus = "interrupted"
	}
	if _, err := tx.Exec(ctx, `
		UPDATE managed_sessions AS s
		SET status = $3,
		    updated_at = $4,
		    last_event_at = $4,
		    event_count = counts.event_count
		FROM (
			SELECT COUNT(*)::integer AS event_count
			FROM managed_session_events
			WHERE tenant_id = $1 AND session_id = $2
		) AS counts
		WHERE s.tenant_id = $1 AND s.session_id = $2
	`, tenantID, task.SessionID, sessionStatus, eventCreatedAt); err != nil {
		return task, err
	}

	if err := tx.Commit(ctx); err != nil {
		return task, err
	}
	_ = s.syncManagedTaskRun(ctx, tenantID, task)
	return task, nil
}

func (s *PostgresStore) syncManagedTaskRun(ctx context.Context, tenantID string, task models.ManagedTask) error {
	status := "running"
	var endTime any
	switch strings.TrimSpace(task.Status) {
	case "completed":
		status = "success"
		if task.CompletedAt != nil {
			endTime = task.CompletedAt.UTC()
		} else {
			endTime = task.UpdatedAt.UTC()
		}
	case "denied", "failed", "error", "cancelled":
		status = "error"
		if task.CompletedAt != nil {
			endTime = task.CompletedAt.UTC()
		} else {
			endTime = task.UpdatedAt.UTC()
		}
	}

	metadata := mustJSON(map[string]any{
		"source":     "managed_runtime_task",
		"session_id": task.SessionID,
		"task_id":    task.ID,
	})
	_, err := s.pool.Exec(ctx, `
		INSERT INTO runs (
			run_id, trace_id, parent_run_id, framework, agent_name, model,
			start_time, end_time, status, total_tokens, total_cost_usd, metadata, tenant_id
		)
		SELECT
			$1,
			$2,
			NULLIF($3, ''),
			'anthropic_managed',
			COALESCE(NULLIF(a.name, ''), ms.agent_id),
			COALESCE(NULLIF(a.model, ''), ''),
			$4,
			$5,
			$6,
			$7,
			$8,
			$9::jsonb,
			$10
		FROM managed_sessions ms
		LEFT JOIN managed_agents a
			ON a.tenant_id = ms.tenant_id AND a.agent_id = ms.agent_id
		WHERE ms.tenant_id = $10 AND ms.session_id = $2
		ON CONFLICT (run_id, tenant_id) DO UPDATE SET
			trace_id = EXCLUDED.trace_id,
			parent_run_id = EXCLUDED.parent_run_id,
			framework = EXCLUDED.framework,
			agent_name = EXCLUDED.agent_name,
			model = EXCLUDED.model,
			start_time = LEAST(runs.start_time, EXCLUDED.start_time),
			end_time = EXCLUDED.end_time,
			status = EXCLUDED.status,
			total_tokens = EXCLUDED.total_tokens,
			total_cost_usd = EXCLUDED.total_cost_usd,
			metadata = EXCLUDED.metadata
	`, task.ID, task.SessionID, strings.TrimSpace(task.ParentTaskID), task.StartedAt.UTC(), endTime, status, task.TotalTokens, task.TotalCostUSD, metadata, tenantID)
	return err
}

func isManagedTaskTerminal(status string) bool {
	switch strings.TrimSpace(status) {
	case "completed", "failed", "error", "cancelled", "denied":
		return true
	default:
		return false
	}
}

func mustJSON(value any) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}
