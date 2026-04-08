package prompts

import (
	"context"
	"testing"
	"time"

	"github.com/govagn/api-gateway/internal/models"
)

func TestAttachCurrentReleases(t *testing.T) {
	versions := []models.PromptVersion{
		{PromptID: "support", Version: 2, Environment: "production"},
		{PromptID: "support", Version: 1, Environment: "development"},
	}
	releases := []models.PromptRelease{
		{PromptID: "support", Version: 2, Environment: "production", ReleaseTag: "2026.03"},
	}

	attachCurrentReleases(versions, releases)

	if versions[0].CurrentRelease == nil || versions[0].CurrentRelease.ReleaseTag != "2026.03" {
		t.Fatalf("expected current release to be attached: %#v", versions[0])
	}
	if !versions[0].Promoted {
		t.Fatalf("expected promoted flag on matching version")
	}
	if versions[1].CurrentRelease != nil {
		t.Fatalf("did not expect current release for development version")
	}
}

func TestReleaseKey(t *testing.T) {
	if got := releaseKey("support", "production"); got != "support::production" {
		t.Fatalf("unexpected release key %q", got)
	}
}

type fakePromptStore struct {
	versions    []models.PromptVersion
	releases    []models.PromptRelease
	upserted    models.PromptVersion
	upsertErr   error
	selected    models.PromptVersion
	selectedErr error
	promoted    models.PromptRelease
	promoteErr  error
	releaseRuns map[string][]models.TraceEvalRun
}

func (f *fakePromptStore) ListPromptVersions(context.Context, string) ([]models.PromptVersion, error) {
	return f.versions, nil
}

func (f *fakePromptStore) ListPromptReleases(context.Context, string) ([]models.PromptRelease, error) {
	return f.releases, nil
}

func (f *fakePromptStore) UpsertPromptVersion(_ context.Context, _ string, version models.PromptVersion) (models.PromptVersion, error) {
	if f.upsertErr != nil {
		return models.PromptVersion{}, f.upsertErr
	}
	f.upserted = version
	version.Version = 3
	return version, nil
}

func (f *fakePromptStore) GetPromptVersion(context.Context, string, string, int) (models.PromptVersion, error) {
	return f.selected, f.selectedErr
}

func (f *fakePromptStore) PromotePromptRelease(_ context.Context, _ string, release models.PromptRelease) (models.PromptRelease, error) {
	if f.promoteErr != nil {
		return models.PromptRelease{}, f.promoteErr
	}
	f.promoted = release
	release.ID = 22
	return release, nil
}

func (f *fakePromptStore) ListEvalRunsByRelease(_ context.Context, _ string, promptID, environment, releaseTag, _ string) ([]models.TraceEvalRun, error) {
	if f.releaseRuns == nil {
		return nil, nil
	}
	return f.releaseRuns[promptID+"::"+environment+"::"+releaseTag], nil
}

func TestListCatalogAttachesCurrentRelease(t *testing.T) {
	fake := &fakePromptStore{
		versions: []models.PromptVersion{
			{PromptID: "support", Version: 2, Environment: "production"},
		},
		releases: []models.PromptRelease{
			{PromptID: "support", Version: 2, Environment: "production", ReleaseTag: "2026.03", Status: "active"},
		},
		releaseRuns: map[string][]models.TraceEvalRun{
			"support::production::2026.03": {
				{OverallScore: 91.5, RiskLevel: "low", CreatedAt: time.Unix(1700000000, 0).UTC()},
			},
		},
	}
	svc := &Service{store: fake}
	catalog, err := svc.ListCatalog(context.Background(), "tenant-1")
	if err != nil {
		t.Fatalf("ListCatalog returned error: %v", err)
	}
	if catalog.Count != 1 || catalog.Items[0].CurrentRelease == nil {
		t.Fatalf("unexpected catalog: %#v", catalog)
	}
	if catalog.Releases[0].EvalSummary.EvalCount != 1 || catalog.Releases[0].EvalSummary.AverageScore != 91.5 {
		t.Fatalf("expected eval summary to be attached, got %#v", catalog.Releases[0].EvalSummary)
	}
}

func TestUpsertVersionValidatesAndNormalizesFields(t *testing.T) {
	fake := &fakePromptStore{}
	svc := &Service{store: fake}

	version, err := svc.UpsertVersion(context.Background(), "tenant-1", " architect ", models.PromptVersion{
		PromptID:    " support.system ",
		Environment: " Production ",
		Content:     "  be helpful ",
		Description: "  desc ",
	})
	if err != nil {
		t.Fatalf("UpsertVersion returned error: %v", err)
	}
	if fake.upserted.PromptID != "support.system" || fake.upserted.Environment != "production" || fake.upserted.CreatedBy != "architect" {
		t.Fatalf("unexpected upsert payload: %#v", fake.upserted)
	}
	if version.Version != 3 {
		t.Fatalf("expected store result to be returned, got %#v", version)
	}
}

func TestPromoteBuildsReleaseFromSelectedVersion(t *testing.T) {
	fake := &fakePromptStore{
		selected: models.PromptVersion{
			PromptID: "support.system",
			Version:  4,
		},
	}
	svc := &Service{store: fake}
	release, err := svc.Promote(context.Background(), "tenant-1", "owner", models.PromptPromotionRequest{
		PromptID:        " support.system ",
		Environment:     " Production ",
		Version:         4,
		ReleaseTag:      " 2026.04 ",
		Status:          " candidate ",
		Notes:           " promote ",
		PromotionReason: " improve escalation quality ",
	})
	if err != nil {
		t.Fatalf("Promote returned error: %v", err)
	}
	if fake.promoted.Environment != "production" || fake.promoted.PromotedBy != "owner" {
		t.Fatalf("unexpected promoted payload: %#v", fake.promoted)
	}
	if fake.promoted.Status != "candidate" || fake.promoted.PromotionReason != "improve escalation quality" {
		t.Fatalf("expected promotion metadata to be normalized, got %#v", fake.promoted)
	}
	if release.ID != 22 {
		t.Fatalf("expected stored release to be returned, got %#v", release)
	}
}
