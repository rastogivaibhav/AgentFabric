use crate::span::Span;

/// Result of evaluating a span against all active policy rules.
#[derive(Debug, Clone)]
pub struct PolicyResult {
    /// "allow" | "review" | "deny"
    pub decision: &'static str,
    /// Whether PII indicators were found in span attributes
    pub pii_detected: bool,
    /// Human-readable reason for non-allow decisions
    pub reason: Option<String>,
}

// ── PII keyword heuristics ──────────────────────────────────────────────────
// These are checked against both attribute keys and values (lowercased).
// The collector already runs regex-based PII scrubbing; this is a second-pass
// heuristic layer that catches structural indicators the regex may miss.
const PII_ATTRIBUTE_KEYS: &[&str] = &[
    "password",
    "passwd",
    "secret",
    "api_key",
    "apikey",
    "private_key",
    "bearer",
    "auth_token",
    "access_token",
    "credit_card",
    "card_number",
    "ssn",
    "social_security",
    "passport",
    "dob",
    "date_of_birth",
];

// Tokens above this threshold trigger a "review" decision regardless of content
const HIGH_TOKEN_THRESHOLD: i64 = 100_000;

/// Evaluate all policy rules for a span and return the aggregate decision.
/// The most restrictive decision wins: deny > review > allow.
pub fn evaluate(span: &Span) -> PolicyResult {
    let mut pii_detected = false;
    let mut reasons: Vec<String> = Vec::new();

    // Rule 1: PII key detection in attribute names
    for key in span.attributes.keys() {
        let lower = key.to_lowercase();
        if PII_ATTRIBUTE_KEYS.iter().any(|k| lower.contains(k)) {
            pii_detected = true;
            reasons.push(format!("pii_key:{}", key));
            break;
        }
    }

    // Rule 2: PII keyword detection in attribute values (first 512 chars each)
    if !pii_detected {
        'outer: for (_key, value) in &span.attributes {
            let lower = value[..value.len().min(512)].to_lowercase();
            for kw in PII_ATTRIBUTE_KEYS {
                if lower.contains(kw) {
                    pii_detected = true;
                    reasons.push("pii_value_match".to_string());
                    break 'outer;
                }
            }
        }
    }

    // Rule 3: Unusually high token count — flag for human review
    if span.input_tokens > HIGH_TOKEN_THRESHOLD {
        reasons.push(format!("high_input_tokens:{}", span.input_tokens));
        return PolicyResult {
            decision: "review",
            pii_detected,
            reason: Some(reasons.join(",")),
        };
    }

    // Rule 4: Error spans with status_code == 2 (OTel ERROR status)
    // These pass through as "allow" but the flag enables error-rate dashboards
    // in ClickHouse without requiring a separate policy pipeline.

    if pii_detected {
        PolicyResult {
            decision: "review",
            pii_detected: true,
            reason: Some(reasons.join(",")),
        }
    } else {
        PolicyResult {
            decision: "allow",
            pii_detected: false,
            reason: None,
        }
    }
}
