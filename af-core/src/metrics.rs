// af-core/src/metrics.rs
use prometheus::{Counter, Opts, Registry, register_counter};

lazy_static::lazy_static! {
    pub static ref AUDIT_WRITE_FAILURES: Counter = register_counter!(
        Opts::new("af_core_audit_write_failures_total",
                  "Number of audit log write failures — CRITICAL if > 0")
    ).unwrap();

    pub static ref SPANS_PROCESSED: Counter = register_counter!(
        Opts::new("af_core_spans_processed_total", "Total spans processed by af-core")
    ).unwrap();

    pub static ref POLICY_DENIALS: Counter = register_counter!(
        Opts::new("af_core_policy_denials_total", "Total policy deny decisions")
    ).unwrap();
}
