// af-core/src/policy/audit_tests.rs
// Unit tests for the hash-chained audit log (Principle 4 — immutable audit trail).

#[cfg(test)]
mod tests {
    use sha2::{Digest, Sha256};
    use uuid::Uuid;
    use chrono::Utc;

    use crate::policy::engine::{PolicyDecision, PolicyResult};

    // ─── Hash chain helpers (mirrors AuditWriter logic) ───────────────────────

    fn compute_entry_hash(decision: &PolicyDecision, prev_hash: &str) -> String {
        let payload = format!(
            "{}:{}:{}:{}:{}:{}",
            decision.decision_id,
            decision.trace_id,
            decision.policy_name,
            decision.result,
            decision.evaluated_at.timestamp_nanos_opt().unwrap_or(0),
            prev_hash,
        );
        let mut hasher = Sha256::new();
        hasher.update(payload.as_bytes());
        hex::encode(hasher.finalize())
    }

    fn make_decision(policy_name: &str, result: PolicyResult) -> PolicyDecision {
        PolicyDecision {
            decision_id:   Uuid::new_v4(),
            tenant_id:     "tenant-001".to_string(),
            trace_id:      "trace-abc".to_string(),
            span_id:       "span-xyz".to_string(),
            policy_name:   policy_name.to_string(),
            result,
            reason:        "test reason".to_string(),
            evaluated_at:  Utc::now(),
            evaluated_ns:  100,
            previous_hash: String::new(),
            entry_hash:    String::new(),
        }
    }

    // ─── SHA256 chain computation tests ──────────────────────────────────────

    #[test]
    fn test_first_entry_chains_from_genesis() {
        let mut d = make_decision("test.policy", PolicyResult::Allow);
        let genesis = "genesis";
        let hash = compute_entry_hash(&d, genesis);

        // Must be 64-char lowercase hex (SHA256)
        assert_eq!(hash.len(), 64);
        assert!(hash.chars().all(|c| c.is_ascii_hexdigit()));

        d.previous_hash = genesis.to_string();
        d.entry_hash = hash.clone();
        assert_eq!(d.previous_hash, "genesis");
        assert!(!d.entry_hash.is_empty());
    }

    #[test]
    fn test_chain_links_entries_sequentially() {
        let mut entries: Vec<PolicyDecision> = Vec::new();
        let mut prev_hash = "genesis".to_string();

        for i in 0..5 {
            let result = if i % 2 == 0 { PolicyResult::Allow } else { PolicyResult::Warn };
            let mut d = make_decision(&format!("policy.{}", i), result);
            let hash = compute_entry_hash(&d, &prev_hash);
            d.previous_hash = prev_hash.clone();
            d.entry_hash = hash.clone();
            prev_hash = hash;
            entries.push(d);
        }

        // Verify sequential linking
        assert_eq!(entries[0].previous_hash, "genesis");
        assert_eq!(entries[1].previous_hash, entries[0].entry_hash);
        assert_eq!(entries[2].previous_hash, entries[1].entry_hash);
        assert_eq!(entries[3].previous_hash, entries[2].entry_hash);
        assert_eq!(entries[4].previous_hash, entries[3].entry_hash);
    }

    #[test]
    fn test_chain_verification_detects_tampering() {
        let mut entries: Vec<PolicyDecision> = Vec::new();
        let mut prev_hash = "genesis".to_string();

        for i in 0..4 {
            let mut d = make_decision(&format!("policy.{}", i), PolicyResult::Allow);
            let hash = compute_entry_hash(&d, &prev_hash);
            d.previous_hash = prev_hash.clone();
            d.entry_hash = hash.clone();
            prev_hash = hash;
            entries.push(d);
        }

        // Tamper with entry 2 — change the reason but keep old hash
        entries[2].reason = "TAMPERED_REASON".to_string();

        // Replay verification: walk from genesis, recompute each hash
        let mut replay_prev = "genesis".to_string();
        let mut broken_at = None;
        for (i, entry) in entries.iter().enumerate() {
            let expected = compute_entry_hash(entry, &replay_prev);
            if expected != entry.entry_hash {
                broken_at = Some(i);
                break;
            }
            replay_prev = entry.entry_hash.clone();
        }

        // Tampering in entry 2 must be detected
        assert!(broken_at.is_some(), "chain verification should detect tampering");
        assert_eq!(broken_at.unwrap(), 2);
    }

    #[test]
    fn test_chain_verification_passes_clean_chain() {
        let mut entries: Vec<PolicyDecision> = Vec::new();
        let mut prev_hash = "genesis".to_string();

        for i in 0..10 {
            let result = match i % 4 {
                0 => PolicyResult::Allow,
                1 => PolicyResult::Warn,
                2 => PolicyResult::Deny,
                _ => PolicyResult::Sanitize,
            };
            let mut d = make_decision(&format!("policy.{}", i), result);
            let hash = compute_entry_hash(&d, &prev_hash);
            d.previous_hash = prev_hash.clone();
            d.entry_hash = hash.clone();
            prev_hash = hash;
            entries.push(d);
        }

        // Verify clean chain
        let mut replay_prev = "genesis".to_string();
        let mut broken_at = None;
        for (i, entry) in entries.iter().enumerate() {
            let expected = compute_entry_hash(entry, &replay_prev);
            if expected != entry.entry_hash {
                broken_at = Some(i);
                break;
            }
            replay_prev = entry.entry_hash.clone();
        }

        assert!(broken_at.is_none(), "clean chain should pass verification at entry {:?}", broken_at);
    }

    #[test]
    fn test_empty_chain_is_valid() {
        // An empty audit log is valid (no entries to check)
        let entries: Vec<PolicyDecision> = Vec::new();
        let mut broken_at = None;
        let mut replay_prev = "genesis".to_string();
        for (i, entry) in entries.iter().enumerate() {
            let expected = compute_entry_hash(entry, &replay_prev);
            if expected != entry.entry_hash {
                broken_at = Some(i);
                break;
            }
            replay_prev = entry.entry_hash.clone();
        }
        assert!(broken_at.is_none());
    }

    #[test]
    fn test_hash_is_deterministic() {
        let d = make_decision("deterministic.test", PolicyResult::Allow);
        let h1 = compute_entry_hash(&d, "genesis");
        let h2 = compute_entry_hash(&d, "genesis");
        assert_eq!(h1, h2, "SHA256 must be deterministic");
    }

    #[test]
    fn test_different_policies_produce_different_hashes() {
        let d1 = make_decision("policy.A", PolicyResult::Allow);
        let d2 = make_decision("policy.B", PolicyResult::Allow);
        let h1 = compute_entry_hash(&d1, "genesis");
        let h2 = compute_entry_hash(&d2, "genesis");
        assert_ne!(h1, h2, "different policy names must produce different hashes");
    }

    #[test]
    fn test_different_results_produce_different_hashes() {
        let d_allow = make_decision("policy.X", PolicyResult::Allow);
        let d_deny  = make_decision("policy.X", PolicyResult::Deny);
        let h1 = compute_entry_hash(&d_allow, "genesis");
        let h2 = compute_entry_hash(&d_deny, "genesis");
        assert_ne!(h1, h2, "different results must produce different hashes");
    }

    #[test]
    fn test_different_prev_hash_produces_different_hash() {
        let d = make_decision("policy.Y", PolicyResult::Allow);
        let h1 = compute_entry_hash(&d, "genesis");
        let h2 = compute_entry_hash(&d, "aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666aaaa1111bbbb2222");
        assert_ne!(h1, h2, "different previous hashes must produce different entry hashes");
    }

    #[test]
    fn test_deleted_entry_breaks_chain() {
        let mut entries: Vec<PolicyDecision> = Vec::new();
        let mut prev_hash = "genesis".to_string();

        for i in 0..5 {
            let mut d = make_decision(&format!("policy.{}", i), PolicyResult::Allow);
            let hash = compute_entry_hash(&d, &prev_hash);
            d.previous_hash = prev_hash.clone();
            d.entry_hash = hash.clone();
            prev_hash = hash;
            entries.push(d);
        }

        // Simulate deletion of entry 2 by removing it
        entries.remove(2);
        assert_eq!(entries.len(), 4);

        // Chain is now broken at index 2 (previously entry 3)
        let mut replay_prev = "genesis".to_string();
        let mut broken_at = None;
        for (i, entry) in entries.iter().enumerate() {
            let expected = compute_entry_hash(entry, &replay_prev);
            if expected != entry.entry_hash {
                broken_at = Some(i);
                break;
            }
            replay_prev = entry.entry_hash.clone();
        }

        assert!(broken_at.is_some(), "deleting an entry must break the chain");
        assert_eq!(broken_at.unwrap(), 2);
    }

    // ─── Principle 4 compliance ───────────────────────────────────────────────

    #[test]
    fn test_p4_audit_chain_entry_hash_is_not_empty() {
        let mut d = make_decision("p4.compliance", PolicyResult::Deny);
        let hash = compute_entry_hash(&d, "genesis");
        d.entry_hash = hash;

        // Principle 4: audit entries must always have a non-empty hash
        assert!(!d.entry_hash.is_empty(), "Principle 4: entry_hash must never be empty");
        assert!(!d.previous_hash.is_empty() || d.previous_hash == "genesis",
            "Principle 4: previous_hash must be set before persisting");
    }

    #[test]
    fn test_p4_chain_is_insert_only_no_update_possible() {
        // The hash-chain design itself enforces this — any modification
        // to an existing entry changes its hash, breaking the chain from that point forward.
        // This test documents and verifies that invariant.
        let mut entries: Vec<PolicyDecision> = Vec::new();
        let mut prev_hash = "genesis".to_string();
        for i in 0..3 {
            let mut d = make_decision(&format!("policy.{}", i), PolicyResult::Allow);
            let hash = compute_entry_hash(&d, &prev_hash);
            d.previous_hash = prev_hash.clone();
            d.entry_hash = hash.clone();
            prev_hash = hash;
            entries.push(d);
        }

        // "Update" entry 1 result — this would be an illegal operation
        let original_hash = entries[1].entry_hash.clone();
        entries[1].result = PolicyResult::Deny;
        let new_hash = compute_entry_hash(&entries[1], &entries[1].previous_hash);

        // The hash changes → the stored hash is now wrong → chain breaks
        assert_ne!(original_hash, new_hash,
            "Modifying result must invalidate stored hash (proving insert-only semantics)");
    }
}
