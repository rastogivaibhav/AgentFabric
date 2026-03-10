// af-core/src/storage/postgres.rs
// PostgreSQL storage layer using sqlx
// Multi-tenant with Row Level Security (RT-010 fix)

use anyhow::Result;
use chrono::{DateTime, Utc};
use sqlx::{PgPool, postgres::PgPoolOptions, Row};
use tracing::{info, error};
use uuid::Uuid;

use crate::pipeline::span::EnrichedSpan;
use crate::policy::engine::PolicyDecision;
use crate::policy::audit::ChainVerificationResult;

#[derive(Clone)]
pub struct PostgresStore {
    pool: PgPool,
}

impl PostgresStore {
    pub async fn connect(url: &str) -> Result<Self> {
        let pool = PgPoolOptions::new()
            .max_connections(20)
            .min_connections(2)
            .acquire_timeout(std::time::Duration::from_secs(10))
            .connect(url)
            .await?;
        info!("PostgreSQL connected");
        Ok(Self { pool })
    }

    pub async fn run_migrations(&self) -> Result<()> {
        sqlx::migrate!("./migrations").run(&self.pool).await?;
        info!("Database migrations applied");
        Ok(())
    }

    // ─── Span Upsert ───────────────────────────────────────────────────────

    pub async fn upsert_span(&self, span: &EnrichedSpan) -> Result<()> {
        // Set tenant context for RLS (RT-010)
        sqlx::query("SET LOCAL app.tenant_id = $1")
            .bind(&span.tenant_id)
            .execute(&self.pool)
            .await?;

        sqlx::query(r#"
            INSERT INTO span_metadata (
                span_id, trace_id, tenant_id, parent_span_id,
                name, framework, span_kind,
                model, tool_name, tool_status,
                start_ns, end_ns, duration_ms, status_code, status_message,
                input_tokens, output_tokens, cost_total_usd,
                service_name, environment, cloud_region,
                policy_decision, pii_detected
            ) VALUES (
                $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,
                $11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23
            )
            ON CONFLICT (span_id) DO UPDATE SET
                policy_decision = EXCLUDED.policy_decision,
                pii_detected    = EXCLUDED.pii_detected
        "#)
        .bind(&span.span_id)
        .bind(&span.trace_id)
        .bind(&span.tenant_id)
        .bind(&span.parent_span_id)
        .bind(&span.name)
        .bind(&span.framework)
        .bind(&span.span_kind)
        .bind(&span.model)
        .bind(&span.tool_name)
        .bind(&span.tool_status)
        .bind(span.start_time_ns as i64)
        .bind((span.start_time_ns + span.duration_ns) as i64)
        .bind(span.duration_ms())
        .bind(span.status_code)
        .bind(&span.status_msg)
        .bind(span.input_tokens)
        .bind(span.output_tokens)
        .bind(span.cost_usd)
        .bind(&span.service_name)
        .bind(&span.environment)
        .bind(&span.cloud_region)
        .bind(&span.policy_decision)
        .bind(span.pii_detected)
        .execute(&self.pool)
        .await?;

        Ok(())
    }

    // ─── Agent Run Upsert ──────────────────────────────────────────────────

    pub async fn upsert_agent_run(&self, span: &EnrichedSpan) -> Result<()> {
        if !span.is_root() { return Ok(()); }

        sqlx::query("SET LOCAL app.tenant_id = $1")
            .bind(&span.tenant_id)
            .execute(&self.pool)
            .await?;

        sqlx::query(r#"
            INSERT INTO agent_runs (
                trace_id, tenant_id, framework, agent_name,
                model, environment, service_name, host_name, cloud_region,
                start_time, status, input_tokens, output_tokens,
                cost_input_usd, cost_output_usd
            ) VALUES (
                $1,$2,$3,$4,$5,$6,$7,$8,$9,
                to_timestamp($10::bigint / 1e9),
                'running', $11,$12,$13,$14
            )
            ON CONFLICT (trace_id) DO UPDATE SET
                status        = CASE WHEN EXCLUDED.status = 'running' THEN agent_runs.status ELSE EXCLUDED.status END,
                input_tokens  = agent_runs.input_tokens  + EXCLUDED.input_tokens,
                output_tokens = agent_runs.output_tokens + EXCLUDED.output_tokens
        "#)
        .bind(&span.trace_id)
        .bind(&span.tenant_id)
        .bind(&span.framework)
        .bind(&span.agent_name)
        .bind(&span.model)
        .bind(&span.environment)
        .bind(&span.service_name)
        .bind(&span.host_name)
        .bind(&span.cloud_region)
        .bind(span.start_time_ns as i64)
        .bind(span.input_tokens)
        .bind(span.output_tokens)
        .bind(span.cost_usd * 0.6) // approximate split
        .bind(span.cost_usd * 0.4)
        .execute(&self.pool)
        .await?;

        Ok(())
    }

    // ─── Audit Log (INSERT ONLY — RT-009) ─────────────────────────────────

    pub async fn insert_audit_entry(&self, d: &PolicyDecision) -> Result<()> {
        sqlx::query(r#"
            INSERT INTO policy_audit_log (
                decision_id, tenant_id, trace_id, span_id,
                policy_name, result, reason,
                evaluated_at, evaluated_ns,
                previous_hash, entry_hash
            ) VALUES (
                $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11
            )
        "#)
        .bind(d.decision_id)
        .bind(&d.tenant_id)
        .bind(&d.trace_id)
        .bind(&d.span_id)
        .bind(&d.policy_name)
        .bind(d.result.to_string())
        .bind(&d.reason)
        .bind(d.evaluated_at)
        .bind(d.evaluated_ns)
        .bind(&d.previous_hash)
        .bind(&d.entry_hash)
        .execute(&self.pool)
        .await?;

        Ok(())
    }

    pub async fn load_audit_entries(&self, tenant_id: &str) -> Result<Vec<PolicyDecision>> {
        // Returns entries ordered by id ASC for chain verification
        let rows = sqlx::query(r#"
            SELECT decision_id, tenant_id, trace_id, span_id,
                   policy_name, result, reason,
                   evaluated_at, evaluated_ns,
                   previous_hash, entry_hash
            FROM policy_audit_log
            WHERE tenant_id = $1
            ORDER BY id ASC
        "#)
        .bind(tenant_id)
        .fetch_all(&self.pool)
        .await?;

        let entries = rows.iter().map(|r| PolicyDecision {
            decision_id:   r.get::<Uuid, _>("decision_id"),
            tenant_id:     r.get("tenant_id"),
            trace_id:      r.get("trace_id"),
            span_id:       r.get("span_id"),
            policy_name:   r.get("policy_name"),
            result:        match r.get::<String, _>("result").as_str() {
                "deny"     => crate::policy::engine::PolicyResult::Deny,
                "warn"     => crate::policy::engine::PolicyResult::Warn,
                "sanitize" => crate::policy::engine::PolicyResult::Sanitize,
                _          => crate::policy::engine::PolicyResult::Allow,
            },
            reason:        r.get("reason"),
            evaluated_at:  r.get("evaluated_at"),
            evaluated_ns:  r.get("evaluated_ns"),
            previous_hash: r.get("previous_hash"),
            entry_hash:    r.get("entry_hash"),
        }).collect();

        Ok(entries)
    }

    // ─── Query API ─────────────────────────────────────────────────────────

    pub async fn query_spans(
        &self,
        tenant_id: &str,
        limit: i64,
        offset: i64,
        framework: Option<&str>,
        since_ns: Option<i64>,
    ) -> Result<Vec<serde_json::Value>> {
        sqlx::query("SET LOCAL app.tenant_id = $1")
            .bind(tenant_id)
            .execute(&self.pool)
            .await?;

        let fw_filter = framework.map(|f| format!("AND framework = '{}'", f)).unwrap_or_default();
        let time_filter = since_ns.map(|t| format!("AND start_ns >= {}", t)).unwrap_or_default();

        let sql = format!(r#"
            SELECT span_id, trace_id, parent_span_id, name,
                   framework, span_kind, model, tool_name,
                   duration_ms, status_code, input_tokens, output_tokens,
                   cost_total_usd, environment, cloud_region,
                   policy_decision, pii_detected,
                   to_timestamp(start_ns::bigint / 1e9) AS start_time
            FROM span_metadata
            WHERE tenant_id = $1
            {} {}
            ORDER BY start_ns DESC
            LIMIT $2 OFFSET $3
        "#, fw_filter, time_filter);

        let rows = sqlx::query(&sql)
            .bind(tenant_id)
            .bind(limit)
            .bind(offset)
            .fetch_all(&self.pool)
            .await?;

        let spans: Vec<serde_json::Value> = rows.iter().map(|r| {
            serde_json::json!({
                "span_id":        r.get::<String, _>("span_id"),
                "trace_id":       r.get::<String, _>("trace_id"),
                "name":           r.get::<String, _>("name"),
                "framework":      r.get::<String, _>("framework"),
                "span_kind":      r.get::<String, _>("span_kind"),
                "model":          r.get::<String, _>("model"),
                "duration_ms":    r.get::<f64, _>("duration_ms"),
                "status_code":    r.get::<i32, _>("status_code"),
                "policy_decision": r.get::<String, _>("policy_decision"),
                "cost_usd":       r.get::<f64, _>("cost_total_usd"),
            })
        }).collect();

        Ok(spans)
    }
}
