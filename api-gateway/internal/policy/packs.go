package policy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/govagn/api-gateway/internal/models"
	"gopkg.in/yaml.v3"
)

const NativePackRegoModule = "native_pack"

type PackCatalog struct {
	SchemaVersion string `yaml:"schema_version" json:"schema_version"`
	Catalog       struct {
		ID          string                  `yaml:"id" json:"id"`
		Version     string                  `yaml:"version" json:"version"`
		GeneratedAt string                  `yaml:"generated_at" json:"generated_at"`
		Packs       []PackCatalogEntry      `yaml:"packs" json:"packs"`
		Profiles    []PackCatalogProfileRef `yaml:"profiles" json:"profiles"`
	} `yaml:"catalog" json:"catalog"`
}

type PackCatalogEntry struct {
	ID       string `yaml:"id" json:"id"`
	File     string `yaml:"file" json:"file"`
	Category string `yaml:"category" json:"category"`
}

type PackCatalogProfileRef struct {
	Name string `yaml:"name" json:"name"`
	File string `yaml:"file" json:"file"`
}

type PackDefinition struct {
	SchemaVersion string                 `yaml:"schema_version" json:"schema_version"`
	Pack          PackMetadata           `yaml:"pack" json:"pack"`
	Defaults      PackDefaults           `yaml:"defaults" json:"defaults"`
	Definitions   PackDefinitions        `yaml:"definitions" json:"definitions"`
	Policies      []PackPolicyDefinition `yaml:"policies" json:"policies"`
	Controls      map[string]any         `yaml:"controls" json:"controls"`
	Approvals     map[string]any         `yaml:"approvals" json:"approvals"`
}

type PackMetadata struct {
	ID            string   `yaml:"id" json:"id"`
	Name          string   `yaml:"name" json:"name"`
	Version       string   `yaml:"version" json:"version"`
	Jurisdiction  []string `yaml:"jurisdiction" json:"jurisdiction"`
	Owner         string   `yaml:"owner" json:"owner"`
	Status        string   `yaml:"status" json:"status"`
	EffectiveFrom string   `yaml:"effective_from" json:"effective_from"`
	Tags          []string `yaml:"tags" json:"tags"`
}

type PackDefaults struct {
	ConflictResolution string `yaml:"conflict_resolution" json:"conflict_resolution"`
	OnUnknown          string `yaml:"on_unknown" json:"on_unknown"`
	EvidenceMode       string `yaml:"evidence_mode" json:"evidence_mode"`
	DecisionTTLSeconds int    `yaml:"decision_ttl_seconds" json:"decision_ttl_seconds"`
}

type PackDefinitions struct {
	DataClasses []string `yaml:"data_classes" json:"data_classes"`
	RiskLevels  []string `yaml:"risk_levels" json:"risk_levels"`
}

type PackPolicyDefinition struct {
	ID        string                 `yaml:"id" json:"id"`
	Title     string                 `yaml:"title" json:"title"`
	AppliesTo PackAppliesTo          `yaml:"applies_to" json:"applies_to"`
	Conditions map[string]any        `yaml:"conditions" json:"conditions"`
	Decision  PackDecision           `yaml:"decision" json:"decision"`
	Obligations []map[string]any     `yaml:"obligations" json:"obligations"`
	Severity  string                 `yaml:"severity" json:"severity"`
}

type PackAppliesTo struct {
	Actions     []string `yaml:"actions" json:"actions"`
	DataClasses []string `yaml:"data_classes" json:"data_classes"`
}

type PackDecision struct {
	IfFalse string `yaml:"if_false" json:"if_false"`
	IfTrue  string `yaml:"if_true" json:"if_true"`
}

type PackDeploymentProfile struct {
	DeploymentProfile struct {
		Name  string   `yaml:"name" json:"name"`
		Packs []string `yaml:"packs" json:"packs"`
	} `yaml:"deployment_profile" json:"deployment_profile"`
	Resolution map[string]any `yaml:"resolution" json:"resolution"`
	Runtime    map[string]any `yaml:"runtime" json:"runtime"`
}

type NativePackRuleEnvelope struct {
	SchemaVersion string               `json:"schema_version"`
	Pack          PackMetadata         `json:"pack"`
	Defaults      PackDefaults         `json:"defaults,omitempty"`
	Definitions   PackDefinitions      `json:"definitions,omitempty"`
	Policy        PackPolicyDefinition `json:"policy"`
	Controls      map[string]any       `json:"controls,omitempty"`
	Approvals     map[string]any       `json:"approvals,omitempty"`
}

type NativePackRuleEvaluation struct {
	Action      string
	Reason      string
	Matched     bool
	Unsupported []string
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

func LoadPackDefinitions(root string) ([]PackDefinition, error) {
	catalog, err := LoadPackCatalog(root)
	if err != nil {
		return nil, err
	}
	byFile := map[string]struct{}{}
	for _, entry := range catalog.Catalog.Packs {
		byFile[entry.File] = struct{}{}
	}
	files := make([]string, 0, len(byFile))
	for file := range byFile {
		files = append(files, file)
	}
	sort.Strings(files)
	packs := make([]PackDefinition, 0, len(files))
	for _, file := range files {
		items, err := loadPackFile(filepath.Join(root, file))
		if err != nil {
			return nil, err
		}
		packs = append(packs, items...)
	}
	return packs, nil
}

func GetPackDefinition(root, packID string) (PackDefinition, error) {
	packs, err := LoadPackDefinitions(root)
	if err != nil {
		return PackDefinition{}, err
	}
	for _, pack := range packs {
		if strings.TrimSpace(pack.Pack.ID) == strings.TrimSpace(packID) {
			return pack, nil
		}
	}
	return PackDefinition{}, fmt.Errorf("pack not found: %s", packID)
}

func loadPackFile(path string) ([]PackDefinition, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	packs := []PackDefinition{}
	for {
		var pack PackDefinition
		err := decoder.Decode(&pack)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(pack.Pack.ID) == "" {
			continue
		}
		packs = append(packs, pack)
	}
	return packs, nil
}

func LoadPackProfiles(root string) ([]PackDeploymentProfile, error) {
	raw, err := os.ReadFile(filepath.Join(root, "profiles.yaml"))
	if err != nil {
		return nil, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	out := []PackDeploymentProfile{}
	for {
		var profile PackDeploymentProfile
		err := decoder.Decode(&profile)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(profile.DeploymentProfile.Name) == "" {
			continue
		}
		out = append(out, profile)
	}
	return out, nil
}

func CompilePackRules(pack PackDefinition, tenantID *string, enabled bool) ([]models.PolicyRule, error) {
	rules := make([]models.PolicyRule, 0, len(pack.Policies))
	for idx, item := range pack.Policies {
		envelope := NativePackRuleEnvelope{
			SchemaVersion: pack.SchemaVersion,
			Pack:          pack.Pack,
			Defaults:      pack.Defaults,
			Definitions:   pack.Definitions,
			Policy:        item,
			Controls:      pack.Controls,
			Approvals:     pack.Approvals,
		}
		schemaJSON, err := json.Marshal(envelope)
		if err != nil {
			return nil, err
		}
		supported := nativePackPolicySupported(item.Conditions)
		rule := models.PolicyRule{
			TenantID:       tenantID,
			Name:           fmt.Sprintf("pack:%s:%s", strings.TrimSpace(pack.Pack.ID), strings.TrimSpace(item.ID)),
			RuleType:       "traffic",
			DecisionMode:   "rego",
			Enabled:        enabled && supported,
			Priority:       nativePackPriority(item.Severity, idx),
			Action:         normalizePackAction(preferredNonPermitAction(item.Decision)),
			Scope:          "both",
			RolloutPercent: 100,
			Version:        1,
			RuleConditions: map[string]string{
				"native_pack_id":       strings.TrimSpace(pack.Pack.ID),
				"native_pack_name":     strings.TrimSpace(pack.Pack.Name),
				"native_policy_id":     strings.TrimSpace(item.ID),
				"native_policy_title":  strings.TrimSpace(item.Title),
				"native_policy_state":  map[bool]string{true: "supported", false: "partial"}[supported],
				"native_failure_hint":  normalizePackAction(preferredNonPermitAction(item.Decision)),
				"native_pack_category": strings.Join(pack.Pack.Tags, ","),
			},
			RegoModule:  NativePackRegoModule,
			SchemaJSON:  string(schemaJSON),
			Description: nativePackDescription(pack, item, supported),
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func nativePackDescription(pack PackDefinition, item PackPolicyDefinition, supported bool) string {
	state := "compiled for native-pack evaluation"
	if !supported {
		state = "stored as native-pack template; some operators need richer runtime context"
	}
	return fmt.Sprintf("%s [%s] from %s (%s)", strings.TrimSpace(item.Title), strings.TrimSpace(item.ID), strings.TrimSpace(pack.Pack.Name), state)
}

func nativePackPriority(severity string, offset int) int {
	base := map[string]int{
		"critical": 980,
		"high":     920,
		"moderate": 860,
		"low":      800,
	}[strings.ToLower(strings.TrimSpace(severity))]
	if base == 0 {
		base = 820
	}
	return base - offset
}

func normalizePackAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "deny":
		return "deny"
	case "redact":
		return "redact"
	case "escalate":
		return "warn"
	case "allow", "permit":
		return "allow"
	default:
		return "warn"
	}
}

func preferredNonPermitAction(decision PackDecision) string {
	if action := strings.TrimSpace(decision.IfFalse); action != "" && !strings.EqualFold(action, "permit") {
		return action
	}
	if action := strings.TrimSpace(decision.IfTrue); action != "" && !strings.EqualFold(action, "permit") {
		return action
	}
	return "warn"
}

func nativePackPolicySupported(condition map[string]any) bool {
	unsupported := nativePackUnsupported(condition)
	return len(unsupported) == 0
}

func nativePackUnsupported(condition map[string]any) []string {
	unsupported := []string{}
	var walk func(map[string]any)
	walk = func(node map[string]any) {
		for key, raw := range node {
			switch strings.TrimSpace(key) {
			case "all", "any", "none":
				items, ok := raw.([]any)
				if !ok {
					unsupported = append(unsupported, key)
					continue
				}
				for _, item := range items {
					child, ok := item.(map[string]any)
					if !ok {
						unsupported = append(unsupported, key)
						continue
					}
					walk(child)
				}
			case "field_exists", "equals", "not_equals", "greater_than", "greater_than_or_equal",
				"less_than", "less_than_or_equal", "min_length", "field_in", "purpose_in",
				"equals_ref", "allowed_fields_subset", "minimum_documents_present", "within_days",
				"not_expired", "tool_in_allowlist", "agent_tier_gte", "same_or_higher_trust_zone":
			default:
				if strings.HasSuffix(key, "_below") || strings.HasSuffix(key, "_above") {
					continue
				}
				switch raw.(type) {
				case string, bool, int, int64, float64, []any:
				default:
					unsupported = append(unsupported, key)
				}
			}
		}
	}
	walk(condition)
	sort.Strings(unsupported)
	return unsupported
}

func evaluateNativePackEnvelope(envelope NativePackRuleEnvelope, input EvaluationInput) NativePackRuleEvaluation {
	if len(input.Attributes) == 0 {
		return NativePackRuleEvaluation{}
	}
	if len(envelope.Policy.Conditions) == 0 {
		return NativePackRuleEvaluation{}
	}
	supported, matched := evalNativeConditionTree(envelope.Policy.Conditions, input.Attributes)
	if !supported {
		return NativePackRuleEvaluation{Unsupported: nativePackUnsupported(envelope.Policy.Conditions)}
	}
	action := strings.TrimSpace(envelope.Policy.Decision.IfFalse)
	if matched {
		action = strings.TrimSpace(envelope.Policy.Decision.IfTrue)
	}
	if action == "" || strings.EqualFold(action, "permit") {
		return NativePackRuleEvaluation{}
	}
	return NativePackRuleEvaluation{
		Action:  normalizePackAction(action),
		Reason:  fmt.Sprintf("%s (%s) produced %s", strings.TrimSpace(envelope.Policy.Title), strings.TrimSpace(envelope.Policy.ID), strings.ToLower(strings.TrimSpace(action))),
		Matched: true,
	}
}

func evalNativeConditionTree(node map[string]any, attrs map[string]any) (bool, bool) {
	if len(node) == 0 {
		return true, false
	}
	if raw, ok := node["all"]; ok {
		items, ok := raw.([]any)
		if !ok {
			return false, false
		}
		for _, item := range items {
			child, ok := item.(map[string]any)
			if !ok {
				return false, false
			}
			supported, matched := evalNativeConditionTree(child, attrs)
			if !supported {
				return false, false
			}
			if !matched {
				return true, false
			}
		}
		return true, true
	}
	if raw, ok := node["any"]; ok {
		items, ok := raw.([]any)
		if !ok {
			return false, false
		}
		seenSupported := false
		for _, item := range items {
			child, ok := item.(map[string]any)
			if !ok {
				return false, false
			}
			supported, matched := evalNativeConditionTree(child, attrs)
			if !supported {
				return false, false
			}
			seenSupported = true
			if matched {
				return true, true
			}
		}
		return seenSupported, false
	}
	if raw, ok := node["none"]; ok {
		items, ok := raw.([]any)
		if !ok {
			return false, false
		}
		for _, item := range items {
			child, ok := item.(map[string]any)
			if !ok {
				return false, false
			}
			supported, matched := evalNativeConditionTree(child, attrs)
			if !supported {
				return false, false
			}
			if matched {
				return true, false
			}
		}
		return true, true
	}
	for key, raw := range node {
		return evalNativeLeaf(key, raw, attrs)
	}
	return true, false
}

func evalNativeLeaf(operator string, raw any, attrs map[string]any) (bool, bool) {
	switch operator {
	case "field_exists":
		path, ok := raw.(string)
		if !ok {
			return false, false
		}
		_, exists := resolvePackPath(path, attrs)
		return true, exists
	case "equals":
		field, expected, ok := unpackFieldValue(raw)
		if !ok {
			return false, false
		}
		actual, exists := resolvePackPath(field, attrs)
		return true, exists && compareEqual(actual, expected)
	case "not_equals":
		field, expected, ok := unpackFieldValue(raw)
		if !ok {
			return false, false
		}
		actual, exists := resolvePackPath(field, attrs)
		return true, exists && !compareEqual(actual, expected)
	case "greater_than", "greater_than_or_equal", "less_than", "less_than_or_equal":
		field, expected, ok := unpackFieldValue(raw)
		if !ok {
			return false, false
		}
		actual, exists := resolvePackPath(field, attrs)
		if !exists {
			return true, false
		}
		return true, compareNumeric(operator, actual, expected)
	case "min_length":
		field, expected, ok := unpackFieldValue(raw)
		if !ok {
			return false, false
		}
		actual, exists := resolvePackPath(field, attrs)
		if !exists {
			return true, false
		}
		return true, lengthOf(actual) >= int(coerceFloat(expected))
	case "field_in", "purpose_in":
		payload, ok := raw.(map[string]any)
		if !ok {
			return false, false
		}
		field, _ := payload["field"].(string)
		values, ok := toStringSlice(payload["values"])
		if !ok {
			return false, false
		}
		actual, exists := resolvePackPath(field, attrs)
		if !exists {
			return true, false
		}
		return true, stringInSlice(fmt.Sprint(actual), values)
	case "equals_ref":
		payload, ok := raw.(map[string]any)
		if !ok {
			return false, false
		}
		left, _ := payload["left"].(string)
		right, _ := payload["right"].(string)
		leftVal, leftOK := resolvePackPath(left, attrs)
		rightVal, rightOK := resolvePackPath(right, attrs)
		return true, leftOK && rightOK && compareEqual(leftVal, rightVal)
	case "allowed_fields_subset":
		payload, ok := raw.(map[string]any)
		if !ok {
			return false, false
		}
		requestedRef, _ := payload["requested_fields_ref"].(string)
		allowedRef, _ := payload["allowed_profile_ref"].(string)
		requested, okRequested := resolvePackStringSlice(requestedRef, attrs)
		allowed, okAllowed := resolvePackStringSlice(allowedRef, attrs)
		return true, okRequested && okAllowed && isSubset(requested, allowed)
	case "minimum_documents_present":
		required, ok := toStringSlice(raw)
		if !ok {
			return false, false
		}
		actual, ok := resolvePackStringSlice("claim.documents", attrs)
		if !ok {
			actual, ok = resolvePackStringSlice("context.documents", attrs)
		}
		return true, ok && isSubset(required, actual)
	case "within_days":
		field, expected, ok := unpackFieldValue(raw)
		if !ok {
			return false, false
		}
		actual, exists := resolvePackPath(field, attrs)
		if !exists {
			return true, false
		}
		timestamp, ok := coerceTime(actual)
		if !ok {
			return false, false
		}
		return true, time.Since(timestamp.UTC()) <= time.Duration(int(coerceFloat(expected)))*24*time.Hour
	case "not_expired":
		payload, ok := raw.(map[string]any)
		if !ok {
			return false, false
		}
		field, _ := payload["field"].(string)
		actual, exists := resolvePackPath(field, attrs)
		if !exists {
			return true, false
		}
		timestamp, ok := coerceTime(actual)
		if !ok {
			return false, false
		}
		return true, !timestamp.UTC().Before(time.Now().UTC())
	case "tool_in_allowlist":
		payload, ok := raw.(map[string]any)
		if !ok {
			return false, false
		}
		agentRef, _ := payload["agent_ref"].(string)
		toolRef, _ := payload["tool_ref"].(string)
		agentID, okAgent := resolvePackString(agentRef, attrs)
		toolName, okTool := resolvePackString(toolRef, attrs)
		if !okAgent || !okTool {
			return true, false
		}
		value, exists := resolvePackPath("policy_refs.agent_tool_allowlist."+sanitizeMapKey(agentID), attrs)
		if !exists {
			value, exists = resolvePackPath("policy_refs.agent_tool_allowlist", attrs)
			if !exists {
				return true, false
			}
			if boolMap, ok := value.(map[string]any); ok {
				if allowed, ok := boolMap[toolName]; ok {
					return true, truthy(allowed)
				}
			}
			return true, false
		}
		tools, ok := anyToStringSlice(value)
		return true, ok && stringInSlice(toolName, tools)
	case "agent_tier_gte":
		payload, ok := raw.(map[string]any)
		if !ok {
			return false, false
		}
		leftField, _ := payload["agent_field"].(string)
		rightField, _ := payload["required_field"].(string)
		left, leftOK := resolvePackPath(leftField, attrs)
		right, rightOK := resolvePackPath(rightField, attrs)
		return true, leftOK && rightOK && compareOrdered(left, right) >= 0
	case "same_or_higher_trust_zone":
		payload, ok := raw.(map[string]any)
		if !ok {
			return false, false
		}
		leftField, _ := payload["source_field"].(string)
		rightField, _ := payload["target_field"].(string)
		left, leftOK := resolvePackPath(leftField, attrs)
		right, rightOK := resolvePackPath(rightField, attrs)
		return true, leftOK && rightOK && compareOrdered(left, right) >= 0
	default:
		if strings.HasSuffix(operator, "_below") {
			path := inferNumericFieldPath(operator, attrs)
			actual, exists := resolvePackPath(path, attrs)
			if !exists {
				return true, false
			}
			return true, compareNumeric("less_than", actual, raw)
		}
		if strings.HasSuffix(operator, "_above") {
			path := inferNumericFieldPath(operator, attrs)
			actual, exists := resolvePackPath(path, attrs)
			if !exists {
				return true, false
			}
			return true, compareNumeric("greater_than", actual, raw)
		}
		if path, ok := raw.(string); ok {
			value, exists := resolvePackPath(path, attrs)
			return true, exists && truthy(value)
		}
	}
	return false, false
}

func inferNumericFieldPath(operator string, attrs map[string]any) string {
	name := strings.TrimSuffix(strings.TrimSuffix(operator, "_below"), "_above")
	candidates := []string{
		"context." + name,
		"runtime." + name,
		"record." + name,
		"order." + name,
		"claim." + name,
	}
	for _, path := range candidates {
		if _, ok := resolvePackPath(path, attrs); ok {
			return path
		}
	}
	return candidates[0]
}

func unpackFieldValue(raw any) (string, any, bool) {
	payload, ok := raw.(map[string]any)
	if !ok {
		return "", nil, false
	}
	field, _ := payload["field"].(string)
	value, ok := payload["value"]
	return field, value, field != "" && ok
}

func resolvePackPath(path string, attrs map[string]any) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false
	}
	segments := strings.Split(path, ".")
	var current any = attrs
	for _, segment := range segments {
		switch typed := current.(type) {
		case map[string]any:
			value, ok := typed[segment]
			if !ok {
				value, ok = typed[sanitizeMapKey(segment)]
				if !ok {
					return nil, false
				}
			}
			current = value
		case map[string]string:
			value, ok := typed[segment]
			if !ok {
				return nil, false
			}
			current = value
		default:
			return nil, false
		}
	}
	return current, true
}

func sanitizeMapKey(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, ".", "_")
	value = strings.ReplaceAll(value, "-", "_")
	return value
}

func resolvePackString(path string, attrs map[string]any) (string, bool) {
	value, ok := resolvePackPath(path, attrs)
	if !ok {
		return "", false
	}
	return fmt.Sprint(value), true
}

func resolvePackStringSlice(path string, attrs map[string]any) ([]string, bool) {
	value, ok := resolvePackPath(path, attrs)
	if !ok {
		return nil, false
	}
	return anyToStringSlice(value)
}

func anyToStringSlice(value any) ([]string, bool) {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...), true
	case []any:
		return toStringSlice(typed)
	default:
		return nil, false
	}
}

func toStringSlice(value any) ([]string, bool) {
	items, ok := value.([]any)
	if !ok {
		if direct, ok := value.([]string); ok {
			return append([]string(nil), direct...), true
		}
		return nil, false
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, fmt.Sprint(item))
	}
	return out, true
}

func compareEqual(actual, expected any) bool {
	switch exp := expected.(type) {
	case bool:
		return truthy(actual) == exp
	default:
		return strings.EqualFold(strings.TrimSpace(fmt.Sprint(actual)), strings.TrimSpace(fmt.Sprint(expected)))
	}
}

func compareNumeric(operator string, actual, expected any) bool {
	left := coerceFloat(actual)
	right := coerceFloat(expected)
	switch operator {
	case "greater_than":
		return left > right
	case "greater_than_or_equal":
		return left >= right
	case "less_than":
		return left < right
	case "less_than_or_equal":
		return left <= right
	default:
		return false
	}
}

func coerceFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case int32:
		return float64(typed)
	case json.Number:
		out, _ := typed.Float64()
		return out
	default:
		out, _ := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(value)), 64)
		return out
	}
}

func coerceTime(value any) (time.Time, bool) {
	switch typed := value.(type) {
	case time.Time:
		return typed, true
	case string:
		for _, layout := range []string{time.RFC3339, "2006-01-02"} {
			if parsed, err := time.Parse(layout, strings.TrimSpace(typed)); err == nil {
				return parsed, true
			}
		}
	}
	return time.Time{}, false
}

func lengthOf(value any) int {
	switch typed := value.(type) {
	case string:
		return len(typed)
	case []string:
		return len(typed)
	case []any:
		return len(typed)
	default:
		return 0
	}
}

func truthy(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes", "on", "passed", "clear", "approved":
			return true
		}
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	}
	return false
}

func stringInSlice(target string, values []string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func isSubset(requested, allowed []string) bool {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		allowedSet[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}
	for _, value := range requested {
		if _, ok := allowedSet[strings.ToLower(strings.TrimSpace(value))]; !ok {
			return false
		}
	}
	return true
}

func compareOrdered(left, right any) int {
	if li, lok := tryInt(left); lok {
		if ri, rok := tryInt(right); rok {
			switch {
			case li > ri:
				return 1
			case li < ri:
				return -1
			default:
				return 0
			}
		}
	}
	order := map[string]int{
		"low": 1, "moderate": 2, "medium": 2, "high": 3, "critical": 4,
		"bronze": 1, "silver": 2, "gold": 3, "platinum": 4,
		"untrusted": 1, "trusted": 2, "privileged": 3,
	}
	lv := order[strings.ToLower(strings.TrimSpace(fmt.Sprint(left)))]
	rv := order[strings.ToLower(strings.TrimSpace(fmt.Sprint(right)))]
	switch {
	case lv > rv:
		return 1
	case lv < rv:
		return -1
	default:
		return 0
	}
}

func tryInt(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), true
	case string:
		out, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return out, err == nil
	default:
		return 0, false
	}
}
