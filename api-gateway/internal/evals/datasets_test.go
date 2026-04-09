package evals

import (
	"path/filepath"
	"testing"
)

func TestLoadEvalDatasetDefinitions(t *testing.T) {
	root := filepath.Join("..", "..", "..", "deploy", "seed", "eval-datasets")
	items, err := LoadDatasetDefinitions(root)
	if err != nil {
		t.Fatalf("load eval datasets: %v", err)
	}
	if len(items) < 4 {
		t.Fatalf("expected seeded eval datasets, got %d", len(items))
	}
}
