package evals

import (
	"path/filepath"
	"testing"
)

func TestLoadEvalPackDefinitions(t *testing.T) {
	root := filepath.Join("..", "..", "..", "deploy", "seed", "eval-packs")
	packs, err := LoadPackDefinitions(root)
	if err != nil {
		t.Fatalf("load eval packs: %v", err)
	}
	if len(packs) < 10 {
		t.Fatalf("expected seeded eval packs, got %d", len(packs))
	}
}
