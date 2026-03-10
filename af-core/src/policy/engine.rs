// af-core/src/policy/engine.rs
// Production policy engine — all rules evaluated server-side
// Results written to hash-chained audit log (RT-009 fix)

use anyhow::Result;
use chrono::Utc;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use uuid::Uuid;

use super::audit::AuditWriter;
use crate::config::Config;
use crate::pipeline::span::EnrichedSpan;

// ─── Policy Decision ─────────────────────────────────────────────────────────

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "lowercase")]
pub enum PolicyResult {
    Allow,
    Deny,
    Warn,
    Sanitize,
}

impl std::fmt::Display for PolicyResult {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            PolicyResult::Allow    => write!(f, "allow"),
            PolicyResult::Deny     => write!(f, "deny"),
            PolicyResult::Warn     => write!(f, "warn"),
            PolicyResult::Sanitize => write!(f, "sanitize"),
        }
    }
}

#[derive(Debug, Clone, Serialize)]
pub struct PolicyDecision {
    pub decision_id:   Uuid,
    pub tenant_id:     String,
    pub trace_id:      String,
    pub span_id:       String,
    pub policy_name:   String,
    pub result:        PolicyResult,
    pub reason:        String,
    pub evaluated_at:  chrono::DateTime<Utc>,
    pub evaluated_ns:  i64,
    // Hash chain (RT-009)
    pub previous_hash: String,
    pub entry_hash:    String,
}

// ─── Policy Trait ─────────────────────────────────────────────────────────────

pub trait Policy: Send + Sync {
    fn name(&self) -> &str;
    fn applies_to(&self, span: &EnrichedSpan) -> bool;
    fn evaluate(&self, span: &EnrichedSpan) -> (PolicyResult, String);
}

// ─── Built-in Policies ────────────────────────────────────────────────────────

/// Sovereignty: data must stay in allowed cloud regions
pub struct SovereigntyPolicy {
    pub allowed_regions: Vec<String>,
}

impl Policy for SovereigntyPolicy {
    fn name(&self) -> &str { "sovereignty.cross_border_data_flow" }

    fn applies_to(&self, span: &EnrichedSpan) -> bool {
        !span.cloud_region.is_empty()
    }

    fn evaluate(&self, span: &EnrichedSpan) -> (PolicyResult, String) {
        if self.allowed_regions.contains(&span.cloud_region) {
            (PolicyResult::Allow, format!("region '{}' is permitted", span.cloud_region))
        } else {
            (
                PolicyResult::Deny,
                format!(
                    "region '{}' violates data sovereignty — allowed: {:?}",
                    span.cloud_region, self.allowed_regions
                ),
            )
        }
    }
}

/// Cost threshold: warn when a single span's attributed cost is high
pub struct CostThresholdPolicy {
    pub max_cost_usd: f64,
}

impl Policy for CostThresholdPolicy {
    fn name(&self) -> &str { "cost.threshold_exceeded" }

    fn applies_to(&self, span: &EnrichedSpan) -> bool {
        span.cost_usd > 0.0
    }

    fn evaluate(&self, span: &EnrichedSpan) -> (PolicyResult, String) {
        if span.cost_usd > self.max_cost_usd {
            (
                PolicyResult::Warn,
                format!(
                    "span cost ${:.4} exceeds threshold ${:.2}",
                    span.cost_usd, self.max_cost_usd
                ),
            )
        } else {
            (PolicyResult::Allow, "cost within threshold".into())
        }
    }
}

/// Unauthorized tool: deny execution of prohibited tools
pub struct UnauthorizedToolPolicy {
    pub denied_tools: Vec<String>,
}

impl Policy for UnauthorizedToolPolicy {
    fn name(&self) -> &str { "tool.unauthorized_usage" }

    fn applies_to(&self, span: &EnrichedSpan) -> bool {
        !span.tool_name.is_empty()
    }

    fn evaluate(&self, span: &EnrichedSpan) -> (PolicyResult, String) {
        if self.denied_tools.contains(&span.tool_name) {
            (
                PolicyResult::Deny,
                format!("tool '{}' is not authorised for agent execution", span.tool_name),
            )
        } else {
            (PolicyResult::Allow, format!("tool '{}' is permitted", span.tool_name))
        }
    }
}

/// PII in output: flag spans where PII was detected
pub struct PIIOutputPolicy;

impl Policy for PIIOutputPolicy {
    fn name(&self) -> &str { "pii.detected_in_llm_output" }

    fn applies_to(&self, span: &EnrichedSpan) -> bool {
        span.span_kind == "llm_call"
    }

    fn evaluate(&self, span: &EnrichedSpan) -> (PolicyResult, String) {
        if span.attributes.get("af.pii.detected").map(|v| v == "true").unwrap_or(false) {
            (PolicyResult::Warn, "PII detected in LLM output — attributes redacted by collector".into())
        } else {
            (PolicyResult::Allow, "no PII detected".into())
        }
    }
}

/// Rate limit: detect span flooding per service
pub struct RateLimitPolicy {
    pub max_spans_per_min: u64,
    counts: tokio::sync::Mutex<HashMap<String, (u64, std::time::Instant)>>,
}

impl RateLimitPolicy {
    pub fn new(max: u64) -> Self {
        Self {
            max_spans_per_min: max,
            counts: tokio::sync::Mutex::new(HashMap::new()),
        }
    }
}

impl Policy for RateLimitPolicy {
    fn name(&self) -> &str { "rate_limit.span_flood_detection" }
    fn applies_to(&self, _: &EnrichedSpan) -> bool { true }

    fn evaluate(&self, span: &EnrichedSpan) -> (PolicyResult, String) {
        // Non-blocking best-effort check
        if let Ok(mut counts) = self.counts.try_lock() {
            let entry = counts
                .entry(span.service_name.clone())
                .or_insert((0, std::time::Instant::now()));

            if entry.1.elapsed() > std::time::Duration::from_secs(60) {
                *entry = (0, std::time::Instant::now());
            }

            entry.0 += 1;

            if entry.0 > self.max_spans_per_min {
                return (
                    PolicyResult::Warn,
                    format!(
                        "service '{}' submitted {} spans/min — potential flood",
                        span.service_name, entry.0
                    ),
                );
            }
        }
        (PolicyResult::Allow, "within rate limit".into())
    }
}

// ─── Policy Engine ────────────────────────────────────────────────────────────

pub struct PolicyEngine {
    policies:     Vec<Box<dyn Policy>>,
    audit_writer: AuditWriter,
}

impl PolicyEngine {
    pub fn new(cfg: Config, audit_writer: AuditWriter) -> Self {
        let policies: Vec<Box<dyn Policy>> = vec![
            Box::new(SovereigntyPolicy {
                allowed_regions: cfg.allowed_regions.clone(),
            }),
            Box::new(CostThresholdPolicy {
                max_cost_usd: cfg.cost_threshold_usd,
            }),
            Box::new(UnauthorizedToolPolicy {
                denied_tools: cfg.denied_tools.clone(),
            }),
            Box::new(PIIOutputPolicy),
            Box::new(RateLimitPolicy::new(cfg.rate_limit_spans_per_min)),
        ];

        Self { policies, audit_writer }
    }

    /// Evaluate all applicable policies for a span.
    /// SECURITY: Always called server-side. Results are authoritative.
    /// Writes every decision to the hash-chained audit log.
    pub async fn evaluate_all(
        &self,
        span: &EnrichedSpan,
        tenant_id: &str,
    ) -> Result<Vec<PolicyDecision>> {
        let mut decisions = Vec::new();

        for policy in &self.policies {
            if !policy.applies_to(span) {
                continue;
            }

            let start = std::time::Instant::now();
            let (result, reason) = policy.evaluate(span);
            let elapsed_ns = start.elapsed().as_nanos() as i64;

            let mut decision = PolicyDecision {
                decision_id:   Uuid::new_v4(),
                tenant_id:     tenant_id.to_string(),
                trace_id:      span.trace_id.clone(),
                span_id:       span.span_id.clone(),
                policy_name:   policy.name().to_string(),
                result:        result.clone(),
                reason:        reason.clone(),
                evaluated_at:  Utc::now(),
                evaluated_ns:  elapsed_ns,
                previous_hash: String::new(),
                entry_hash:    String::new(),
            };

            // Hash-chain: compute and record (RT-009 fix)
            self.audit_writer.chain_and_append(&mut decision).await?;

            decisions.push(decision);
        }

        Ok(decisions)
    }

    /// Aggregate result: worst-case across all decisions
    pub fn aggregate_result(decisions: &[PolicyDecision]) -> PolicyResult {
        if decisions.iter().any(|d| d.result == PolicyResult::Deny) {
            return PolicyResult::Deny;
        }
        if decisions.iter().any(|d| d.result == PolicyResult::Warn) {
            return PolicyResult::Warn;
        }
        if decisions.iter().any(|d| d.result == PolicyResult::Sanitize) {
            return PolicyResult::Sanitize;
        }
        PolicyResult::Allow
    }
}
