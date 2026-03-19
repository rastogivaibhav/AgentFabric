// af-core/tests/integration_test.rs
//
// Integration tests for the af-core policy evaluation pipeline.
//
// These tests require a live PostgreSQL instance.  In CI the database is
// provided by the GitHub Actions service container defined in .github/workflows/ci.yml:
//
//   TEST_DATABASE_URL=postgres://fabric:fabric@localhost:5432/agentfabric_test
//
// When TEST_DATABASE_URL is unset the tests skip gracefully rather than panic,
// so local development without Docker does not break the test run.
//
// Each test uses a randomly-generated tenant_id (UUID) so all tests are
// fully isolated and can run in parallel without interfering.
//
// Run the full suite:
//   TEST_DATABASE_URL=postgres://... cargo test --test integration_test

use std::collections::HashMap;
use chrono::Utc;
use uuid::Uuid;

use af_core::pipeline::span::EnrichedSpan;
use af_core::policy::audit::AuditWriter;
use af_core::policy::engine::{PolicyDecision, PolicyResult, PolicyEngine};
use af_core::storage::postgres::PostgresStore;
use af_core::config::Config; // used in test_config()

/// Build a minimal Config for integration tests, overriding the given fields.
fn test_config(allowed_regions: Vec<String>, denied_tools: Vec<String>) -> Config {
    Config {
        database_url:              std::env::var("TEST_DATABASE_URL")
                                       .unwrap_or_else(|_| "postgres://fabric:fabric@localhost:5432/agentfabric_test".into()),
        clickhouse_url:            "http://localhost:8123".into(),
        redis_url:                 "redis://localhost:6379/0".into(),
        grpc_addr:                 "0.0.0.0:50051".into(),
        http_addr:                 "0.0.0.0:8889".into(),
        kafka_enabled:             false,
        kafka_brokers:             "localhost:9092".into(),
        kafka_topic:               "af.spans".into(),
        default_tenant_id:         "test".into(),
        allowed_regions,
        cost_threshold_usd:        1.00,
        denied_tools,
        rate_limit_spans_per_min:  10_000,
    }
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

/// Connect to the test database, or return None to skip the test.
async fn connect() -> Option<PostgresStore> {
    let url = std::env::var("TEST_DATABASE_URL").ok()?;
    match PostgresStore::connect(&url).await {
        Ok(store) => Some(store),
        Err(e) => {
            eprintln!("integration: could not connect to DB (skipping): {e}");
            None
        }
    }
}

/// Ensure the policy_audit_log table exists.
/// We create it inline rather than relying on sqlx::migrate! so the
/// integration tests are self-contained and do not require the migrations
/// directory to be present when running `cargo test --test integration_test`
/// in isolation.
async fn ensure_schema(store: &PostgresStore) {
    // The table may already exist from a full migration run — `IF NOT EXISTS` is safe.
    sqlx::query(r#"
        CREATE TABLE IF NOT EXISTS policy_audit_log (
            id             BIGSERIAL PRIMARY KEY,
            decision_id    UUID        NOT NULL,
            tenant_id      TEXT        NOT NULL,
            trace_id       TEXT        NOT NULL DEFAULT '',
            span_id        TEXT        NOT NULL DEFAULT '',
            policy_name    TEXT        NOT NULL,
            result         TEXT        NOT NULL,
            reason         TEXT        NOT NULL DEFAULT '',
            evaluated_at   TIMESTAMPTZ NOT NULL,
            evaluated_ns   BIGINT      NOT NULL DEFAULT 0,
            previous_hash  TEXT        NOT NULL DEFAULT '',
            entry_hash     TEXT        NOT NULL DEFAULT ''
        )
    "#)
    .execute(store.pool())
    .await
    .expect("create policy_audit_log");
}

/// Build a minimal EnrichedSpan for testing.
fn make_span(tenant_id: &str) -> EnrichedSpan {
    EnrichedSpan {
        trace_id:        Uuid::new_v4().to_string(),
        span_id:         Uuid::new_v4().to_string(),
        parent_span_id:  String::new(),
        name:            "integration-test-span".into(),
        framework:       "crewai".into(),
        span_kind:       "llm_call".into(),
        start_time_ns:   1_000_000_000,
        duration_ns:     500_000_000,
        status_code:     0,
        status_msg:      String::new(),
        attributes:      HashMap::new(),
        model:           "claude-3-5-sonnet".into(),
        agent_name:      "test-agent".into(),
        tool_name:       String::new(),
        tool_status:     String::new(),
        run_id:          Uuid::new_v4().to_string(),
        input_tokens:    100,
        output_tokens:   50,
        cost_usd:        0.002,
        input_cost_usd:  0.0012,
        output_cost_usd: 0.0008,
        service_name:    "svc-integration".into(),
        environment:     "test".into(),
        cloud_region:    "eu-west-1".into(),
        host_name:       "host-test".into(),
        collector_node:  "collector-test".into(),
        received_ns:     1_000_000_001,
        policy_decision: "allow".into(),
        pii_detected:    false,
        tenant_id:       tenant_id.into(),
    }
}

/// Build a minimal PolicyDecision for testing.
fn make_decision(tenant_id: &str, result: PolicyResult) -> PolicyDecision {
    PolicyDecision {
        decision_id:   Uuid::new_v4(),
        tenant_id:     tenant_id.into(),
        trace_id:      Uuid::new_v4().to_string(),
        span_id:       Uuid::new_v4().to_string(),
        policy_name:   "test.integration_policy".into(),
        result,
        reason:        "integration test decision".into(),
        evaluated_at:  Utc::now(),
        evaluated_ns:  0,
        previous_hash: String::new(),
        entry_hash:    String::new(),
    }
}

// ─── AuditWriter: basic write-through ────────────────────────────────────────

/// AuditWriter.chain_and_append writes the decision to policy_audit_log
/// and sets entry_hash / previous_hash on the in-memory decision object.
#[tokio::test]
async fn test_audit_writer_persists_decision() {
    let Some(store) = connect().await else { return };
    ensure_schema(&store).await;

    let tenant_id = Uuid::new_v4().to_string();
    let writer    = AuditWriter::new(store.clone());

    let mut decision = make_decision(&tenant_id, PolicyResult::Allow);
    writer.chain_and_append(&mut decision).await
        .expect("chain_and_append should succeed");

    // entry_hash must be populated after the call
    assert!(
        !decision.entry_hash.is_empty(),
        "entry_hash should be set after chain_and_append"
    );
    // previous_hash for the genesis entry must be "genesis"
    assert_eq!(
        decision.previous_hash, "genesis",
        "first entry's previous_hash should be 'genesis'"
    );

    // Read it back from the database
    let entries = store.load_audit_entries(&tenant_id).await
        .expect("load_audit_entries should succeed");

    assert_eq!(entries.len(), 1, "exactly one entry should have been written");
    assert_eq!(entries[0].decision_id, decision.decision_id);
    assert_eq!(entries[0].entry_hash,  decision.entry_hash);
    assert_eq!(entries[0].result,      PolicyResult::Allow);
}

// ─── AuditWriter: hash chain integrity ───────────────────────────────────────

/// Writing N decisions produces a valid, sequential hash chain where each
/// entry's previous_hash equals the prior entry's entry_hash.
#[tokio::test]
async fn test_audit_writer_hash_chain_links_correctly() {
    let Some(store) = connect().await else { return };
    ensure_schema(&store).await;

    let tenant_id = Uuid::new_v4().to_string();
    let writer    = AuditWriter::new(store.clone());

    let results = [
        PolicyResult::Allow,
        PolicyResult::Warn,
        PolicyResult::Deny,
        PolicyResult::Allow,
        PolicyResult::Sanitize,
    ];

    let mut decisions: Vec<PolicyDecision> = Vec::new();
    for result in results {
        let mut d = make_decision(&tenant_id, result);
        writer.chain_and_append(&mut d).await
            .expect("chain_and_append");
        decisions.push(d);
    }

    // Verify the chain links in memory
    for (i, pair) in decisions.windows(2).enumerate() {
        let prev  = &pair[0];
        let curr  = &pair[1];
        assert_eq!(
            curr.previous_hash, prev.entry_hash,
            "entry {} previous_hash should equal entry {} entry_hash",
            i + 1, i
        );
    }

    // Verify via the store's verify_chain method
    let result = writer.verify_chain(&tenant_id).await
        .expect("verify_chain should not error");

    assert!(result.valid, "hash chain should be valid after sequential writes");
    assert_eq!(result.entries_checked, 5, "should have checked 5 entries");
    assert!(result.first_broken_at.is_none(), "no broken link expected");
}

// ─── AuditWriter: verify_chain on empty tenant ───────────────────────────────

/// verify_chain on a tenant with no entries must return valid=true, 0 checked.
#[tokio::test]
async fn test_audit_writer_verify_chain_empty_tenant() {
    let Some(store) = connect().await else { return };
    ensure_schema(&store).await;

    let tenant_id = Uuid::new_v4().to_string(); // never written
    let writer    = AuditWriter::new(store.clone());

    let result = writer.verify_chain(&tenant_id).await
        .expect("verify_chain on empty tenant");

    assert!(result.valid,              "empty chain must be valid");
    assert_eq!(result.entries_checked, 0, "no entries should be checked");
    assert!(result.first_broken_at.is_none());
}

// ─── Multi-tenant isolation ───────────────────────────────────────────────────

/// Entries written for tenant A must not appear when loading tenant B's entries.
#[tokio::test]
async fn test_audit_writer_multi_tenant_isolation() {
    let Some(store) = connect().await else { return };
    ensure_schema(&store).await;

    let tenant_a = Uuid::new_v4().to_string();
    let tenant_b = Uuid::new_v4().to_string();

    let writer_a = AuditWriter::new(store.clone());
    let writer_b = AuditWriter::new(store.clone());

    // Write 3 entries for tenant A, 1 for tenant B
    for _ in 0..3 {
        let mut d = make_decision(&tenant_a, PolicyResult::Allow);
        writer_a.chain_and_append(&mut d).await.expect("write tenant A");
    }
    let mut d_b = make_decision(&tenant_b, PolicyResult::Deny);
    writer_b.chain_and_append(&mut d_b).await.expect("write tenant B");

    let entries_a = store.load_audit_entries(&tenant_a).await
        .expect("load tenant A");
    let entries_b = store.load_audit_entries(&tenant_b).await
        .expect("load tenant B");

    assert_eq!(entries_a.len(), 3, "tenant A should have exactly 3 entries");
    assert_eq!(entries_b.len(), 1, "tenant B should have exactly 1 entry");

    // Ensure no cross-contamination
    for e in &entries_a {
        assert_eq!(e.tenant_id, tenant_a, "tenant A entry must not belong to B");
    }
    for e in &entries_b {
        assert_eq!(e.tenant_id, tenant_b, "tenant B entry must not belong to A");
    }
}

// ─── AuditWriter: deny result persists correctly ─────────────────────────────

#[tokio::test]
async fn test_audit_writer_deny_result_round_trips() {
    let Some(store) = connect().await else { return };
    ensure_schema(&store).await;

    let tenant_id = Uuid::new_v4().to_string();
    let writer    = AuditWriter::new(store.clone());

    let mut d = make_decision(&tenant_id, PolicyResult::Deny);
    d.reason = "tool 'shell_exec' is not authorised".into();
    writer.chain_and_append(&mut d).await.expect("write deny");

    let entries = store.load_audit_entries(&tenant_id).await.expect("load");
    assert_eq!(entries.len(), 1);
    assert_eq!(entries[0].result, PolicyResult::Deny);
    assert!(entries[0].reason.contains("shell_exec"));
}

// ─── PolicyEngine: end-to-end pipeline evaluation ────────────────────────────

/// PolicyEngine.evaluate_all evaluates a span against all built-in policies
/// and writes every applicable decision to the audit log.
#[tokio::test]
async fn test_policy_engine_evaluate_all_writes_to_audit_log() {
    let Some(store) = connect().await else { return };
    ensure_schema(&store).await;

    let tenant_id = Uuid::new_v4().to_string();

    let cfg = test_config(
        vec!["eu-west-1".into()],
        vec!["shell_exec".into()],
    );

    let writer = AuditWriter::new(store.clone());
    let engine = PolicyEngine::new(cfg, writer);

    let span = make_span(&tenant_id);
    let decisions = engine.evaluate_all(&span, &tenant_id).await
        .expect("evaluate_all should not error");

    // At least one policy should have been evaluated (rate_limit applies to all spans)
    assert!(
        !decisions.is_empty(),
        "at least one policy decision must be produced"
    );

    // All decisions must carry the tenant_id (Principle 2: isolation)
    for d in &decisions {
        assert_eq!(d.tenant_id, tenant_id, "every decision must carry the correct tenant_id");
    }

    // All decisions must have been persisted to the DB
    let entries = store.load_audit_entries(&tenant_id).await
        .expect("load audit entries after evaluate_all");

    assert_eq!(
        entries.len(),
        decisions.len(),
        "all {} decisions must be persisted", decisions.len()
    );
}

// ─── PolicyEngine: sovereignty deny is persisted ─────────────────────────────

/// When a span arrives from an out-of-region cloud, the sovereignty policy
/// produces a Deny decision that is persisted to the audit log.
#[tokio::test]
async fn test_policy_engine_sovereignty_deny_persisted() {
    let Some(store) = connect().await else { return };
    ensure_schema(&store).await;

    let tenant_id = Uuid::new_v4().to_string();

    let cfg = test_config(
        vec!["eu-west-1".into()], // us-east-1 will be denied
        vec![],
    );

    let writer = AuditWriter::new(store.clone());
    let engine = PolicyEngine::new(cfg, writer);

    let mut span = make_span(&tenant_id);
    span.cloud_region = "us-east-1".into(); // not in allowed_regions

    let decisions = engine.evaluate_all(&span, &tenant_id).await
        .expect("evaluate_all");

    let sovereignty_decision = decisions.iter()
        .find(|d| d.policy_name == "sovereignty.cross_border_data_flow");

    assert!(
        sovereignty_decision.is_some(),
        "sovereignty policy must produce a decision for non-empty region"
    );
    assert_eq!(
        sovereignty_decision.unwrap().result,
        PolicyResult::Deny,
        "out-of-region span must be denied"
    );

    // Verify the deny appears in the audit log
    let entries = store.load_audit_entries(&tenant_id).await.expect("load");
    let deny_entries: Vec<_> = entries.iter()
        .filter(|e| e.result == PolicyResult::Deny)
        .collect();
    assert!(
        !deny_entries.is_empty(),
        "at least one Deny entry must be in the audit log"
    );
}

// ─── PolicyEngine: aggregate result reflects worst-case ──────────────────────

/// When both allow and deny decisions exist, aggregate_result must return Deny.
#[tokio::test]
async fn test_policy_engine_aggregate_worst_case_is_deny() {
    let Some(store) = connect().await else { return };
    ensure_schema(&store).await;

    let tenant_id = Uuid::new_v4().to_string();

    // Denied tool + in-region span → sovereignty=allow, tool=deny
    let cfg = test_config(
        vec!["eu-west-1".into()],
        vec!["shell_exec".into()],
    );

    let writer = AuditWriter::new(store.clone());
    let engine = PolicyEngine::new(cfg, writer);

    let mut span = make_span(&tenant_id);
    span.tool_name = "shell_exec".into(); // denied

    let decisions = engine.evaluate_all(&span, &tenant_id).await
        .expect("evaluate_all");

    let aggregate = PolicyEngine::aggregate_result(&decisions);
    assert_eq!(
        aggregate,
        PolicyResult::Deny,
        "aggregate must be Deny when any policy produces Deny"
    );
}

// ─── Hash chain: large batch stays intact ────────────────────────────────────

/// Writing 50 decisions in sequence should produce a valid chain.
/// Covers the performance and hash ordering guarantees under bulk load.
#[tokio::test]
async fn test_audit_writer_large_batch_chain_intact() {
    let Some(store) = connect().await else { return };
    ensure_schema(&store).await;

    let tenant_id = Uuid::new_v4().to_string();
    let writer    = AuditWriter::new(store.clone());

    for i in 0..50_u32 {
        let result = match i % 4 {
            0 => PolicyResult::Allow,
            1 => PolicyResult::Warn,
            2 => PolicyResult::Deny,
            _ => PolicyResult::Sanitize,
        };
        let mut d = make_decision(&tenant_id, result);
        writer.chain_and_append(&mut d).await
            .expect("chain_and_append in bulk");
    }

    let verification = writer.verify_chain(&tenant_id).await
        .expect("verify_chain after bulk insert");

    assert!(verification.valid, "50-entry chain must be valid");
    assert_eq!(verification.entries_checked, 50);
    assert!(verification.first_broken_at.is_none());
}
