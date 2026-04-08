package prompts

import (
	"context"
	"fmt"
	"strings"

	"github.com/govagn/api-gateway/internal/models"
	"github.com/govagn/api-gateway/internal/store"
)

type Service struct {
	store promptStore
}

type promptStore interface {
	ListPromptVersions(ctx context.Context, tenantID string) ([]models.PromptVersion, error)
	ListPromptReleases(ctx context.Context, tenantID string) ([]models.PromptRelease, error)
	ListEvalRunsByRelease(ctx context.Context, tenantID, promptID, promptEnvironment, releaseTag, evalSuite string) ([]models.TraceEvalRun, error)
	UpsertPromptVersion(ctx context.Context, tenantID string, version models.PromptVersion) (models.PromptVersion, error)
	GetPromptVersion(ctx context.Context, tenantID, promptID string, versionNum int) (models.PromptVersion, error)
	PromotePromptRelease(ctx context.Context, tenantID string, release models.PromptRelease) (models.PromptRelease, error)
}

func NewService(pg *store.PostgresStore) *Service {
	return &Service{store: pg}
}

func (s *Service) ListCatalog(ctx context.Context, tenantID string) (models.PromptCatalog, error) {
	versions, err := s.store.ListPromptVersions(ctx, tenantID)
	if err != nil {
		return models.PromptCatalog{}, err
	}
	releases, err := s.store.ListPromptReleases(ctx, tenantID)
	if err != nil {
		return models.PromptCatalog{}, err
	}
	if err := attachReleaseHealth(ctx, s.store, tenantID, releases); err != nil {
		return models.PromptCatalog{}, err
	}
	attachCurrentReleases(versions, releases)
	return models.PromptCatalog{
		Items:    versions,
		Releases: releases,
		Count:    len(versions),
	}, nil
}

func (s *Service) UpsertVersion(ctx context.Context, tenantID, actor string, version models.PromptVersion) (models.PromptVersion, error) {
	version.PromptID = strings.TrimSpace(version.PromptID)
	version.Environment = normalizeEnvironment(version.Environment)
	version.ReleaseTag = strings.TrimSpace(version.ReleaseTag)
	version.Content = strings.TrimSpace(version.Content)
	version.Description = strings.TrimSpace(version.Description)
	version.CreatedBy = strings.TrimSpace(actor)
	if version.PromptID == "" {
		return models.PromptVersion{}, fmt.Errorf("prompt_id is required")
	}
	if version.Content == "" {
		return models.PromptVersion{}, fmt.Errorf("content is required")
	}
	return s.store.UpsertPromptVersion(ctx, tenantID, version)
}

func (s *Service) Promote(ctx context.Context, tenantID, actor string, req models.PromptPromotionRequest) (models.PromptRelease, error) {
	req.PromptID = strings.TrimSpace(req.PromptID)
	req.Environment = normalizeEnvironment(req.Environment)
	req.ReleaseTag = strings.TrimSpace(req.ReleaseTag)
	req.Status = normalizeReleaseStatus(req.Status)
	req.Notes = strings.TrimSpace(req.Notes)
	req.PromotionReason = strings.TrimSpace(req.PromotionReason)
	if req.PromptID == "" {
		return models.PromptRelease{}, fmt.Errorf("prompt_id is required")
	}
	if req.Version <= 0 {
		return models.PromptRelease{}, fmt.Errorf("version must be greater than 0")
	}
	if req.ReleaseTag == "" {
		return models.PromptRelease{}, fmt.Errorf("release_tag is required")
	}
	version, err := s.store.GetPromptVersion(ctx, tenantID, req.PromptID, req.Version)
	if err != nil {
		return models.PromptRelease{}, err
	}
	release := models.PromptRelease{
		PromptID:        version.PromptID,
		Environment:     req.Environment,
		Version:         version.Version,
		ReleaseTag:      req.ReleaseTag,
		Status:          req.Status,
		Notes:           req.Notes,
		PromotionReason: req.PromotionReason,
		PromotedBy:      strings.TrimSpace(actor),
	}
	return s.store.PromotePromptRelease(ctx, tenantID, release)
}
