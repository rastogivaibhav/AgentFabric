// af-core/src/policy/audit.rs
// Tamper-evident hash-chained audit log (RT-009 fix)
// Every entry hashes: decision content + previous entry hash → SHA256
// A broken chain is detectable; entries cannot be deleted or reordered silently

use anyhow::Result;
use sha2::{Digest, Sha256};
use tokio::sync::Mutex;
use tracing::{error, warn};

use super::engine::PolicyDecision;
use crate::storage::postgres::PostgresStore;

pub struct AuditWriter {
    store:     PostgresStore,
    last_hash: Mutex<String>,
}

impl AuditWriter {
    pub fn new(store: PostgresStore) -> Self {
        Self {
            store,
            last_hash: Mutex::new("genesis".to_string()),
        }
    }

    /// Compute hash, update chain state, and persist to DB.
    /// The mutex ensures serial ordering even under concurrent evaluation.
    pub async fn chain_and_append(&self, decision: &mut PolicyDecision) -> Result<()> {
        let mut last = self.last_hash.lock().await;

        // Build canonical payload for hashing
        let payload = format!(
            "{}:{}:{}:{}:{}:{}",
            decision.decision_id,
            decision.trace_id,
            decision.policy_name,
            decision.result,
            decision.evaluated_at.timestamp_nanos_opt().unwrap_or(0),
            *last,
        );

        let hash = {
            let mut hasher = Sha256::new();
            hasher.update(payload.as_bytes());
            hex::encode(hasher.finalize())
        };

        decision.previous_hash = last.clone();
        decision.entry_hash    = hash.clone();
        *last = hash;

        // Persist — CRITICAL: if this fails, log loudly but don't panic the pipeline
        match self.store.insert_audit_entry(decision).await {
            Ok(_) => {}
            Err(e) => {
                error!(
                    decision_id = %decision.decision_id,
                    policy = %decision.policy_name,
                    "AUDIT LOG WRITE FAILED — chain integrity at risk: {}",
                    e
                );
                // Increment failure counter (picked up by Prometheus alert)
                crate::metrics::AUDIT_WRITE_FAILURES.inc();
            }
        }

        Ok(())
    }

    /// Verify the hash chain is intact from start to end.
    /// Used by compliance audits and the portal's audit tab.
    pub async fn verify_chain(&self, tenant_id: &str) -> Result<ChainVerificationResult> {
        let entries = self.store.load_audit_entries(tenant_id).await?;

        if entries.is_empty() {
            return Ok(ChainVerificationResult {
                valid: true,
                entries_checked: 0,
                first_broken_at: None,
            });
        }

        let mut prev_hash = "genesis".to_string();
        let mut broken_at = None;

        for (i, entry) in entries.iter().enumerate() {
            let expected_payload = format!(
                "{}:{}:{}:{}:{}:{}",
                entry.decision_id,
                entry.trace_id,
                entry.policy_name,
                entry.result,
                entry.evaluated_at.timestamp_nanos_opt().unwrap_or(0),
                prev_hash,
            );

            let expected_hash = {
                let mut hasher = Sha256::new();
                hasher.update(expected_payload.as_bytes());
                hex::encode(hasher.finalize())
            };

            if expected_hash != entry.entry_hash {
                warn!(
                    entry_index = i,
                    decision_id = %entry.decision_id,
                    "AUDIT CHAIN BROKEN — tampering detected"
                );
                broken_at = Some(i);
                break;
            }

            prev_hash = entry.entry_hash.clone();
        }

        Ok(ChainVerificationResult {
            valid: broken_at.is_none(),
            entries_checked: entries.len(),
            first_broken_at: broken_at,
        })
    }
}

#[derive(Debug, serde::Serialize)]
pub struct ChainVerificationResult {
    pub valid:            bool,
    pub entries_checked:  usize,
    pub first_broken_at:  Option<usize>,
}
