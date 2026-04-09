package evals

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type PackCatalog struct {
	SchemaVersion string `yaml:"schema_version" json:"schema_version"`
	Catalog       struct {
		ID          string              `yaml:"id" json:"id"`
		Version     string              `yaml:"version" json:"version"`
		GeneratedAt string              `yaml:"generated_at" json:"generated_at"`
		Packs       []PackCatalogEntry  `yaml:"packs" json:"packs"`
		Profiles    []PackCatalogProfile `yaml:"profiles" json:"profiles"`
	} `yaml:"catalog" json:"catalog"`
}

type PackCatalogEntry struct {
	ID       string `yaml:"id" json:"id"`
	File     string `yaml:"file" json:"file"`
	Category string `yaml:"category" json:"category"`
}

type PackCatalogProfile struct {
	Name string `yaml:"name" json:"name"`
	File string `yaml:"file" json:"file"`
}

type EvalPackDefinition struct {
	SchemaVersion string                  `yaml:"schema_version" json:"schema_version"`
	Pack          EvalPackMetadata        `yaml:"pack" json:"pack"`
	Defaults      map[string]any          `yaml:"defaults" json:"defaults"`
	Datasets      []map[string]any        `yaml:"datasets" json:"datasets"`
	Dimensions    []EvalPackDimension     `yaml:"dimensions" json:"dimensions"`
	Evaluators    []map[string]any        `yaml:"evaluators" json:"evaluators"`
	Gates         map[string]any          `yaml:"gates" json:"gates"`
	Reporting     map[string]any          `yaml:"reporting" json:"reporting"`
	Approvals     map[string]any          `yaml:"approvals" json:"approvals"`
}

type EvalPackMetadata struct {
	ID            string   `yaml:"id" json:"id"`
	Name          string   `yaml:"name" json:"name"`
	Version       string   `yaml:"version" json:"version"`
	Owner         string   `yaml:"owner" json:"owner"`
	Status        string   `yaml:"status" json:"status"`
	EffectiveFrom string   `yaml:"effective_from" json:"effective_from"`
	Tags          []string `yaml:"tags" json:"tags"`
}

type EvalPackDimension struct {
	ID     string  `yaml:"id" json:"id"`
	Weight float64 `yaml:"weight" json:"weight"`
}

type EvalPackProfile struct {
	DeploymentEvalProfile map[string]any `yaml:"deployment_eval_profile" json:"deployment_eval_profile"`
}

func LoadPackCatalog(root string) (PackCatalog, error) {
	var catalog PackCatalog
	raw, err := os.ReadFile(filepath.Join(root, "catalog.yaml"))
	if err != nil {
		return catalog, err
	}
	if err := yaml.Unmarshal(raw, &catalog); err != nil {
		return catalog, err
	}
	return catalog, nil
}

func LoadPackDefinitions(root string) ([]EvalPackDefinition, error) {
	catalog, err := LoadPackCatalog(root)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	packs := []EvalPackDefinition{}
	for _, entry := range catalog.Catalog.Packs {
		if _, ok := seen[entry.File]; ok {
			continue
		}
		seen[entry.File] = struct{}{}
		items, err := loadPackFile(filepath.Join(root, entry.File))
		if err != nil {
			return nil, err
		}
		packs = append(packs, items...)
	}
	return packs, nil
}

func GetPackDefinition(root, packID string) (EvalPackDefinition, error) {
	packs, err := LoadPackDefinitions(root)
	if err != nil {
		return EvalPackDefinition{}, err
	}
	for _, pack := range packs {
		if strings.TrimSpace(pack.Pack.ID) == strings.TrimSpace(packID) {
			return pack, nil
		}
	}
	return EvalPackDefinition{}, fmt.Errorf("eval pack not found: %s", packID)
}

func loadPackFile(path string) ([]EvalPackDefinition, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	items := []EvalPackDefinition{}
	for {
		var item EvalPackDefinition
		err := decoder.Decode(&item)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(item.Pack.ID) == "" {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func LoadPackProfiles(root string) ([]EvalPackProfile, error) {
	raw, err := os.ReadFile(filepath.Join(root, "profiles.yaml"))
	if err != nil {
		return nil, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	items := []EvalPackProfile{}
	for {
		var item EvalPackProfile
		err := decoder.Decode(&item)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(item.DeploymentEvalProfile) == 0 {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}
