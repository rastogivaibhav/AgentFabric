package prompts

import (
	"context"
	"errors"
	"testing"

	"github.com/agentfabric/api-gateway/internal/models"
)

func TestListCatalogReturnsVersionError(t *testing.T) {
	fakeListErr := errors.New("versions failed")
	svc := &Service{store: promptStoreFuncAdapter{
		listVersions: func(context.Context, string) error { return fakeListErr },
	}}

	_, err := svc.ListCatalog(context.Background(), "tenant-1")
	if !errors.Is(err, fakeListErr) {
		t.Fatalf("expected version error, got %v", err)
	}
}

func TestUpsertVersionRequiresPromptAndContent(t *testing.T) {
	svc := &Service{store: &fakePromptStore{}}

	if _, err := svc.UpsertVersion(context.Background(), "tenant-1", "actor", structWithPrompt("", "content")); err == nil {
		t.Fatalf("expected prompt_id validation error")
	}
	if _, err := svc.UpsertVersion(context.Background(), "tenant-1", "actor", structWithPrompt("prompt", "")); err == nil {
		t.Fatalf("expected content validation error")
	}
}

func TestPromoteValidatesInputs(t *testing.T) {
	svc := &Service{store: &fakePromptStore{}}

	cases := []struct {
		name string
		req  func() string
	}{
		{name: "prompt id", req: func() string {
			_, err := svc.Promote(context.Background(), "tenant-1", "actor", reqWith("", 2, "rel"))
			if err == nil {
				return ""
			}
			return err.Error()
		}},
		{name: "version", req: func() string {
			_, err := svc.Promote(context.Background(), "tenant-1", "actor", reqWith("prompt", 0, "rel"))
			if err == nil {
				return ""
			}
			return err.Error()
		}},
		{name: "release_tag", req: func() string {
			_, err := svc.Promote(context.Background(), "tenant-1", "actor", reqWith("prompt", 2, ""))
			if err == nil {
				return ""
			}
			return err.Error()
		}},
	}

	for _, tc := range cases {
		if got := tc.req(); got == "" {
			t.Fatalf("expected validation error for %s", tc.name)
		}
	}
}

func structWithPrompt(promptID, content string) models.PromptVersion {
	return models.PromptVersion{PromptID: promptID, Content: content}
}

func reqWith(promptID string, version int, tag string) models.PromptPromotionRequest {
	return models.PromptPromotionRequest{
		PromptID:   promptID,
		Version:    version,
		ReleaseTag: tag,
	}
}

type promptStoreFuncAdapter struct {
	listVersions func(context.Context, string) error
}

func (p promptStoreFuncAdapter) ListPromptVersions(ctx context.Context, tenantID string) ([]models.PromptVersion, error) {
	if p.listVersions != nil {
		return nil, p.listVersions(ctx, tenantID)
	}
	return nil, nil
}

func (p promptStoreFuncAdapter) ListPromptReleases(context.Context, string) ([]models.PromptRelease, error) {
	return nil, nil
}

func (p promptStoreFuncAdapter) UpsertPromptVersion(context.Context, string, models.PromptVersion) (models.PromptVersion, error) {
	return models.PromptVersion{}, nil
}

func (p promptStoreFuncAdapter) GetPromptVersion(context.Context, string, string, int) (models.PromptVersion, error) {
	return models.PromptVersion{}, nil
}

func (p promptStoreFuncAdapter) PromotePromptRelease(context.Context, string, models.PromptRelease) (models.PromptRelease, error) {
	return models.PromptRelease{}, nil
}
