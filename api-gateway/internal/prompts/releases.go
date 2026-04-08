package prompts

import "github.com/govagn/api-gateway/internal/models"

func attachCurrentReleases(versions []models.PromptVersion, releases []models.PromptRelease) {
	if len(versions) == 0 || len(releases) == 0 {
		return
	}
	current := make(map[string]models.PromptRelease, len(releases))
	for _, release := range releases {
		key := releaseKey(release.PromptID, release.Environment)
		existing, exists := current[key]
		if !exists || (existing.Status != "active" && release.Status == "active") {
			current[key] = release
		}
	}
	for i := range versions {
		key := releaseKey(versions[i].PromptID, versions[i].Environment)
		if release, ok := current[key]; ok {
			releaseCopy := release
			versions[i].CurrentRelease = &releaseCopy
			versions[i].Promoted = release.Version == versions[i].Version
		}
	}
}

func releaseKey(promptID, environment string) string {
	return promptID + "::" + environment
}
