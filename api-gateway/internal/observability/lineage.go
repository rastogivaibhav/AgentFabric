package observability

import (
	"strings"

	"github.com/agentfabric/api-gateway/internal/models"
)

type lineageMeta struct {
	Depth      int
	ParentName string
	Path       []string
}

func buildLineage(spans []models.Span) map[string]lineageMeta {
	byID := make(map[string]models.Span, len(spans))
	for _, span := range spans {
		byID[span.ID] = span
	}
	memo := make(map[string]lineageMeta, len(spans))
	for _, span := range spans {
		resolveLineage(span.ID, byID, memo, map[string]bool{})
	}
	return memo
}

func resolveLineage(spanID string, byID map[string]models.Span, memo map[string]lineageMeta, seen map[string]bool) lineageMeta {
	if existing, ok := memo[spanID]; ok {
		return existing
	}
	span, ok := byID[spanID]
	if !ok {
		return lineageMeta{}
	}
	if seen[spanID] {
		meta := lineageMeta{Path: []string{labelForSpan(span)}}
		memo[spanID] = meta
		return meta
	}
	seen[spanID] = true
	defer delete(seen, spanID)

	label := labelForSpan(span)
	if strings.TrimSpace(span.ParentID) == "" {
		meta := lineageMeta{
			Depth: 0,
			Path:  []string{label},
		}
		memo[spanID] = meta
		return meta
	}

	parentMeta := resolveLineage(span.ParentID, byID, memo, seen)
	parent := byID[span.ParentID]
	path := append(append([]string{}, parentMeta.Path...), label)
	meta := lineageMeta{
		Depth:      parentMeta.Depth + 1,
		ParentName: parent.Name,
		Path:       path,
	}
	memo[spanID] = meta
	return meta
}

func labelForSpan(span models.Span) string {
	if strings.TrimSpace(span.Name) != "" {
		return strings.TrimSpace(span.Name)
	}
	if strings.TrimSpace(span.ID) != "" {
		return strings.TrimSpace(span.ID)
	}
	return "span"
}
