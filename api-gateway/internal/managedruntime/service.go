package managedruntime

import (
	"context"
	"errors"
	"time"

	"github.com/govagn/api-gateway/internal/models"
	"github.com/jackc/pgx/v5"
)

var ErrNotFound = errors.New("managed runtime resource not found")

type Store interface {
	ListManagedAgents(ctx context.Context, tenantID string, limit int) (*models.Page[models.ManagedAgent], error)
	GetManagedAgent(ctx context.Context, tenantID, agentID string) (models.ManagedAgent, error)
	UpsertManagedAgent(ctx context.Context, tenantID string, agent models.ManagedAgent) (models.ManagedAgent, error)

	ListManagedEnvironments(ctx context.Context, tenantID string, limit int) (*models.Page[models.ManagedEnvironment], error)
	GetManagedEnvironment(ctx context.Context, tenantID, environmentID string) (models.ManagedEnvironment, error)
	UpsertManagedEnvironment(ctx context.Context, tenantID string, environment models.ManagedEnvironment) (models.ManagedEnvironment, error)

	ListManagedSessions(ctx context.Context, tenantID, agentID string, limit int) (*models.Page[models.ManagedSession], error)
	GetManagedSession(ctx context.Context, tenantID, sessionID string) (models.ManagedSession, error)
	CreateManagedSession(ctx context.Context, tenantID string, session models.ManagedSession) (models.ManagedSession, error)

	ListManagedSessionEvents(ctx context.Context, tenantID, sessionID string, limit int) (*models.Page[models.ManagedSessionEvent], error)
	CreateManagedSessionEvent(ctx context.Context, tenantID string, event models.ManagedSessionEvent) (models.ManagedSessionEvent, error)

	ListManagedSessionTasks(ctx context.Context, tenantID, sessionID string, limit int) (*models.Page[models.ManagedTask], error)
	GetManagedTask(ctx context.Context, tenantID, taskID string) (models.ManagedTask, error)
	UpsertManagedTask(ctx context.Context, tenantID string, task models.ManagedTask) (models.ManagedTask, error)

	ListManagedTaskArtifacts(ctx context.Context, tenantID, taskID string, limit int) (*models.Page[models.ManagedArtifact], error)
	CreateManagedArtifact(ctx context.Context, tenantID string, artifact models.ManagedArtifact) (models.ManagedArtifact, error)

	ApproveManagedTask(ctx context.Context, tenantID, taskID, actor, reason string) (models.ManagedTask, error)
	DenyManagedTask(ctx context.Context, tenantID, taskID, actor, reason string) (models.ManagedTask, error)
}

type Service interface {
	ListAgents(ctx context.Context, tenantID string, limit int) (*models.Page[models.ManagedAgent], error)
	GetAgent(ctx context.Context, tenantID, agentID string) (models.ManagedAgent, error)
	UpsertAgent(ctx context.Context, tenantID string, req models.ManagedAgentUpsertRequest) (models.ManagedAgent, error)

	ListEnvironments(ctx context.Context, tenantID string, limit int) (*models.Page[models.ManagedEnvironment], error)
	GetEnvironment(ctx context.Context, tenantID, environmentID string) (models.ManagedEnvironment, error)
	UpsertEnvironment(ctx context.Context, tenantID string, req models.ManagedEnvironmentUpsertRequest) (models.ManagedEnvironment, error)

	ListSessions(ctx context.Context, tenantID, agentID string, limit int) (*models.Page[models.ManagedSession], error)
	GetSession(ctx context.Context, tenantID, sessionID string) (models.ManagedSession, error)
	CreateSession(ctx context.Context, tenantID string, req models.ManagedSessionCreateRequest) (models.ManagedSession, error)

	ListSessionEvents(ctx context.Context, tenantID, sessionID string, limit int) (*models.Page[models.ManagedSessionEvent], error)
	CreateSessionEvent(ctx context.Context, tenantID, sessionID string, req models.ManagedSessionEventCreateRequest) (models.ManagedSessionEvent, error)

	ListSessionTasks(ctx context.Context, tenantID, sessionID string, limit int) (*models.Page[models.ManagedTask], error)
	GetTask(ctx context.Context, tenantID, taskID string) (models.ManagedTask, error)
	UpsertTask(ctx context.Context, tenantID string, req models.ManagedTaskUpsertRequest) (models.ManagedTask, error)

	ListTaskArtifacts(ctx context.Context, tenantID, taskID string, limit int) (*models.Page[models.ManagedArtifact], error)
	CreateArtifact(ctx context.Context, tenantID, taskID string, req models.ManagedArtifactCreateRequest) (models.ManagedArtifact, error)

	ApproveTask(ctx context.Context, tenantID, taskID, actor string, req models.ManagedTaskDecisionRequest) (models.ManagedTask, error)
	DenyTask(ctx context.Context, tenantID, taskID, actor string, req models.ManagedTaskDecisionRequest) (models.ManagedTask, error)
}

type service struct {
	store Store
}

func NewService(store Store) Service {
	return &service{store: store}
}

func (s *service) ListAgents(ctx context.Context, tenantID string, limit int) (*models.Page[models.ManagedAgent], error) {
	return s.store.ListManagedAgents(ctx, tenantID, limit)
}

func (s *service) GetAgent(ctx context.Context, tenantID, agentID string) (models.ManagedAgent, error) {
	item, err := s.store.GetManagedAgent(ctx, tenantID, agentID)
	return item, normalizeNotFound(err)
}

func (s *service) UpsertAgent(ctx context.Context, tenantID string, req models.ManagedAgentUpsertRequest) (models.ManagedAgent, error) {
	return s.store.UpsertManagedAgent(ctx, tenantID, models.ManagedAgent{
		ID:              req.ID,
		Name:            req.Name,
		Description:     req.Description,
		Model:           req.Model,
		SystemPrompt:    req.SystemPrompt,
		RuntimeProvider: req.RuntimeProvider,
		RuntimeProduct:  req.RuntimeProduct,
		Status:          req.Status,
		ToolTypes:       req.ToolTypes,
		MCPServers:      req.MCPServers,
		Skills:          req.Skills,
		EnvironmentIDs:  req.EnvironmentIDs,
		Metadata:        req.Metadata,
		LastSessionAt:   req.LastSessionAt,
	})
}

func (s *service) ListEnvironments(ctx context.Context, tenantID string, limit int) (*models.Page[models.ManagedEnvironment], error) {
	return s.store.ListManagedEnvironments(ctx, tenantID, limit)
}

func (s *service) GetEnvironment(ctx context.Context, tenantID, environmentID string) (models.ManagedEnvironment, error) {
	item, err := s.store.GetManagedEnvironment(ctx, tenantID, environmentID)
	return item, normalizeNotFound(err)
}

func (s *service) UpsertEnvironment(ctx context.Context, tenantID string, req models.ManagedEnvironmentUpsertRequest) (models.ManagedEnvironment, error) {
	return s.store.UpsertManagedEnvironment(ctx, tenantID, models.ManagedEnvironment{
		ID:                req.ID,
		Name:              req.Name,
		Description:       req.Description,
		Runtime:           req.Runtime,
		ContainerTemplate: req.ContainerTemplate,
		NetworkAccess:     req.NetworkAccess,
		MountedFiles:      req.MountedFiles,
		Packages:          req.Packages,
		Status:            req.Status,
		Metadata:          req.Metadata,
	})
}

func (s *service) ListSessions(ctx context.Context, tenantID, agentID string, limit int) (*models.Page[models.ManagedSession], error) {
	return s.store.ListManagedSessions(ctx, tenantID, agentID, limit)
}

func (s *service) GetSession(ctx context.Context, tenantID, sessionID string) (models.ManagedSession, error) {
	item, err := s.store.GetManagedSession(ctx, tenantID, sessionID)
	return item, normalizeNotFound(err)
}

func (s *service) CreateSession(ctx context.Context, tenantID string, req models.ManagedSessionCreateRequest) (models.ManagedSession, error) {
	persistentFilesystem := true
	if req.PersistentFilesystem != nil {
		persistentFilesystem = *req.PersistentFilesystem
	}
	var startedAt time.Time
	if req.StartedAt != nil {
		startedAt = req.StartedAt.UTC()
	}
	return s.store.CreateManagedSession(ctx, tenantID, models.ManagedSession{
		ID:                   req.ID,
		AgentID:              req.AgentID,
		EnvironmentID:        req.EnvironmentID,
		Status:               req.Status,
		CurrentTaskID:        req.CurrentTaskID,
		RuntimeProvider:      req.RuntimeProvider,
		RuntimeProduct:       req.RuntimeProduct,
		PersistentFilesystem: persistentFilesystem,
		ConversationTurns:    req.ConversationTurns,
		EventCount:           req.EventCount,
		Metadata:             req.Metadata,
		StartedAt:            startedAt,
	})
}

func (s *service) ListSessionEvents(ctx context.Context, tenantID, sessionID string, limit int) (*models.Page[models.ManagedSessionEvent], error) {
	return s.store.ListManagedSessionEvents(ctx, tenantID, sessionID, limit)
}

func (s *service) CreateSessionEvent(ctx context.Context, tenantID, sessionID string, req models.ManagedSessionEventCreateRequest) (models.ManagedSessionEvent, error) {
	var createdAt time.Time
	if req.CreatedAt != nil {
		createdAt = req.CreatedAt.UTC()
	}
	return s.store.CreateManagedSessionEvent(ctx, tenantID, models.ManagedSessionEvent{
		ID:        req.ID,
		SessionID: sessionID,
		Type:      req.Type,
		Role:      req.Role,
		Status:    req.Status,
		Sequence:  req.Sequence,
		Summary:   req.Summary,
		Data:      req.Data,
		CreatedAt: createdAt,
	})
}

func (s *service) ListSessionTasks(ctx context.Context, tenantID, sessionID string, limit int) (*models.Page[models.ManagedTask], error) {
	return s.store.ListManagedSessionTasks(ctx, tenantID, sessionID, limit)
}

func (s *service) GetTask(ctx context.Context, tenantID, taskID string) (models.ManagedTask, error) {
	item, err := s.store.GetManagedTask(ctx, tenantID, taskID)
	return item, normalizeNotFound(err)
}

func (s *service) UpsertTask(ctx context.Context, tenantID string, req models.ManagedTaskUpsertRequest) (models.ManagedTask, error) {
	var startedAt time.Time
	if req.StartedAt != nil {
		startedAt = req.StartedAt.UTC()
	}
	return s.store.UpsertManagedTask(ctx, tenantID, models.ManagedTask{
		ID:                 req.ID,
		SessionID:          req.SessionID,
		ParentTaskID:       req.ParentTaskID,
		Status:             req.Status,
		InputSummary:       req.InputSummary,
		OutputSummary:      req.OutputSummary,
		InterruptionReason: req.InterruptionReason,
		ServerToolCount:    req.ServerToolCount,
		ClientToolCount:    req.ClientToolCount,
		TotalCostUSD:       req.TotalCostUSD,
		TotalTokens:        req.TotalTokens,
		Metadata:           req.Metadata,
		StartedAt:          startedAt,
		CompletedAt:        req.CompletedAt,
	})
}

func (s *service) ListTaskArtifacts(ctx context.Context, tenantID, taskID string, limit int) (*models.Page[models.ManagedArtifact], error) {
	return s.store.ListManagedTaskArtifacts(ctx, tenantID, taskID, limit)
}

func (s *service) CreateArtifact(ctx context.Context, tenantID, taskID string, req models.ManagedArtifactCreateRequest) (models.ManagedArtifact, error) {
	return s.store.CreateManagedArtifact(ctx, tenantID, models.ManagedArtifact{
		ID:          req.ID,
		TaskID:      taskID,
		SessionID:   req.SessionID,
		Name:        req.Name,
		Kind:        req.Kind,
		URI:         req.URI,
		ContentType: req.ContentType,
		Status:      req.Status,
		SizeBytes:   req.SizeBytes,
		Metadata:    req.Metadata,
	})
}

func (s *service) ApproveTask(ctx context.Context, tenantID, taskID, actor string, req models.ManagedTaskDecisionRequest) (models.ManagedTask, error) {
	item, err := s.store.ApproveManagedTask(ctx, tenantID, taskID, actor, req.Reason)
	return item, normalizeNotFound(err)
}

func (s *service) DenyTask(ctx context.Context, tenantID, taskID, actor string, req models.ManagedTaskDecisionRequest) (models.ManagedTask, error) {
	item, err := s.store.DenyManagedTask(ctx, tenantID, taskID, actor, req.Reason)
	return item, normalizeNotFound(err)
}

func normalizeNotFound(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
