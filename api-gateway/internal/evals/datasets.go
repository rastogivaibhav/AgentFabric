package evals

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/govagn/api-gateway/internal/models"
	"gopkg.in/yaml.v3"
)

type DatasetCatalog struct {
	SchemaVersion string `yaml:"schema_version" json:"schema_version"`
	Catalog       struct {
		ID          string                `yaml:"id" json:"id"`
		Version     string                `yaml:"version" json:"version"`
		GeneratedAt string                `yaml:"generated_at" json:"generated_at"`
		Datasets    []DatasetCatalogEntry `yaml:"datasets" json:"datasets"`
	} `yaml:"catalog" json:"catalog"`
}

type DatasetCatalogEntry struct {
	Ref      string `yaml:"ref" json:"ref"`
	File     string `yaml:"file" json:"file"`
	Category string `yaml:"category" json:"category"`
}

type DatasetDefinition struct {
	SchemaVersion string `yaml:"schema_version" json:"schema_version"`
	Dataset       struct {
		ID              string         `yaml:"id" json:"id"`
		Version         string         `yaml:"version" json:"version"`
		Ref             string         `yaml:"ref" json:"ref"`
		Name            string         `yaml:"name" json:"name"`
		Type            string         `yaml:"type" json:"type"`
		Owner           string         `yaml:"owner" json:"owner"`
		Status          string         `yaml:"status" json:"status"`
		Source          string         `yaml:"source" json:"source"`
		Description     string         `yaml:"description" json:"description"`
		Provenance      string         `yaml:"provenance" json:"provenance"`
		RedactionStatus string         `yaml:"redaction_status" json:"redaction_status"`
		ApprovalStatus  string         `yaml:"approval_status" json:"approval_status"`
		Tags            []string       `yaml:"tags" json:"tags"`
		Metadata        map[string]any `yaml:"metadata" json:"metadata"`
	} `yaml:"dataset" json:"dataset"`
	Items []struct {
		Key      string         `yaml:"key" json:"key"`
		Input    map[string]any `yaml:"input" json:"input"`
		Expected map[string]any `yaml:"expected" json:"expected"`
		Metadata map[string]any `yaml:"metadata" json:"metadata"`
		Labels   []string       `yaml:"labels" json:"labels"`
	} `yaml:"items" json:"items"`
}

func defaultDatasetRoot() string {
	if configured := strings.TrimSpace(os.Getenv("GV_EVAL_DATASETS_PATH")); configured != "" {
		return configured
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("deploy", "seed", "eval-datasets")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "deploy", "seed", "eval-datasets")
}

func LoadDatasetCatalog(root string) (DatasetCatalog, error) {
	var catalog DatasetCatalog
	raw, err := os.ReadFile(filepath.Join(root, "catalog.yaml"))
	if err != nil {
		return catalog, err
	}
	if err := yaml.Unmarshal(raw, &catalog); err != nil {
		return catalog, err
	}
	return catalog, nil
}

func LoadDatasetDefinitions(root string) ([]models.EvalDataset, error) {
	catalog, err := LoadDatasetCatalog(root)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	out := []models.EvalDataset{}
	for _, entry := range catalog.Catalog.Datasets {
		if _, ok := seen[entry.File]; ok {
			continue
		}
		seen[entry.File] = struct{}{}
		items, err := loadDatasetFile(filepath.Join(root, entry.File))
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}

func GetDatasetDefinition(root, ref string) (models.EvalDataset, error) {
	items, err := LoadDatasetDefinitions(root)
	if err != nil {
		return models.EvalDataset{}, err
	}
	for _, item := range items {
		if strings.TrimSpace(item.Ref) == strings.TrimSpace(ref) {
			return item, nil
		}
	}
	return models.EvalDataset{}, fmt.Errorf("eval dataset not found: %s", ref)
}

func loadDatasetFile(path string) ([]models.EvalDataset, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	out := []models.EvalDataset{}
	for {
		var item DatasetDefinition
		err := decoder.Decode(&item)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(item.Dataset.Ref) == "" {
			continue
		}
		converted := models.EvalDataset{
			DatasetID:       strings.TrimSpace(item.Dataset.ID),
			Version:         strings.TrimSpace(item.Dataset.Version),
			Ref:             strings.TrimSpace(item.Dataset.Ref),
			Name:            strings.TrimSpace(item.Dataset.Name),
			Type:            strings.TrimSpace(item.Dataset.Type),
			Description:     strings.TrimSpace(item.Dataset.Description),
			Owner:           strings.TrimSpace(item.Dataset.Owner),
			Status:          strings.TrimSpace(item.Dataset.Status),
			Source:          firstNonEmpty(item.Dataset.Source, "seed"),
			Provenance:      strings.TrimSpace(item.Dataset.Provenance),
			RedactionStatus: strings.TrimSpace(item.Dataset.RedactionStatus),
			ApprovalStatus:  strings.TrimSpace(item.Dataset.ApprovalStatus),
			Tags:            item.Dataset.Tags,
			Metadata:        item.Dataset.Metadata,
		}
		for _, rawItem := range item.Items {
			converted.Items = append(converted.Items, models.EvalDatasetItem{
				DatasetRef: converted.Ref,
				ItemKey:    strings.TrimSpace(rawItem.Key),
				Input:      rawItem.Input,
				Expected:   rawItem.Expected,
				Metadata:   rawItem.Metadata,
				Labels:     rawItem.Labels,
			})
		}
		out = append(out, converted)
	}
	return out, nil
}
